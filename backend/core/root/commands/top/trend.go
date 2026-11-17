package top

import (
	"context"
	"errors"
	"fmt"
	"krill-agg/root/commands/daily"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/info/theme"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// a moving average's window for a history about `span` days long
type maSpan struct {
	span   int
	window int
	label  string
}

// a history rounds to the nearest span (ties go shorter) and averages over
// its window: a week over 3 days, a month over a week, 3 months over 2 weeks,
// 6 months (or anything longer) over a month
var MA_SPANS = []maSpan{
	{span: 7, window: 3, label: "3-DAY"},
	{span: 30, window: 7, label: "1-WEEK"},
	{span: 91, window: 14, label: "2-WEEK"},
	{span: 182, window: 30, label: "1-MONTH"},
}

func abs(n int) int { return max(n, -n) }

// the window (days) and its label for a history of span days, first game to latest
func maWindow(span int) (int, string) {
	best := MA_SPANS[0]
	for _, m := range MA_SPANS[1:] {
		if abs(m.span-span) < abs(best.span-span) {
			best = m
		}
	}
	return best.window, best.label
}

// one player's games, days ascending
func history(entries []scores.Entry, user string) []scores.PlayerScore {
	var games []scores.PlayerScore
	for _, e := range entries {
		if e.UserID == user {
			games = append(games, e.Score)
		}
	}
	slices.SortFunc(games, func(a, b scores.PlayerScore) int { return a.Day - b.Day })
	return games
}

// as of each game: the mean of the games in the window days ending that day.
// days are calendar days, so days off just leave fewer games to average
func movingAverage(games []scores.PlayerScore, window int) []dailygen.TrendAverage {
	out := make([]dailygen.TrendAverage, len(games))
	from, sum := 0, 0
	for j, g := range games {
		sum += g.Score
		for games[from].Day <= g.Day-window {
			sum -= games[from].Score
			from++
		}
		out[j] = dailygen.TrendAverage{Day: g.Day, Value: float64(sum) / float64(j-from+1)}
	}
	return out
}

// how long the bot waits on dailygen for one trend card
const TREND_TIMEOUT = 5 * time.Second

// a card is named by what it shows: the period and the days its games span.
// a new game (or the period's window leaving an old one behind) is a new
// name, so a new card; anything else (nickname, rank, a corrected score)
// shows up with the next one
func trendName(period string, games []scores.PlayerScore) string {
	return fmt.Sprintf("trend_%s_%d-%d.webp", period, games[0].Day, games[len(games)-1].Day)
}

// a guild's folder of players' cards, each player in their own folder
func trendsDir(bcp *types.BotCommandPackage, guild string) string {
	return filepath.Join(bcp.DataDir, dailygen.DIR, guild, "users")
}

// where a player's card for these games lives
func trendPath(bcp *types.BotCommandPackage, guild, user, period string, games []scores.PlayerScore) string {
	return filepath.Join(trendsDir(bcp, guild), user, trendName(period, games))
}

// removes players' cached cards and links in a guild (every player's when
// users is empty), so their next /topu draws fresh. a player whose card is
// being drawn right now is left alone (busy); cards is how many were removed
func clearTrends(bcp *types.BotCommandPackage, guild string, users []string) (cards int, busy []string, err error) {
	dir := trendsDir(bcp, guild)
	if len(users) == 0 {
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return 0, nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				users = append(users, e.Name())
			}
		}
	}
	for _, user := range users {
		// held while removing, so no draw starts writing into the folder
		_, done, claimErr := claimDraw(guild, user)
		if claimErr != nil {
			busy = append(busy, user)
			continue
		}
		found, _ := filepath.Glob(filepath.Join(dir, user, "trend_*.webp"))
		err = os.RemoveAll(filepath.Join(dir, user))
		done()
		if err != nil {
			return cards, busy, err
		}
		cards += len(found)
	}
	return cards, busy, nil
}

// next to a card: the discord link it was last posted at
func trendLink(path string) string {
	return strings.TrimSuffix(path, ".webp") + ".link"
}

// one draw at a time per player in a guild, whatever the period: guild/user
// -> a link to the reply carrying the draw underway ("" until it's sent)
var (
	drawMu sync.Mutex
	draws  = map[string]string{}
)

// a draw is already underway for the player; link is its reply, if sent yet
type drawingError struct{ link string }

func (e *drawingError) Error() string { return "already generating" }

// claims the player's draw, or says whose reply has it. done frees it;
// publish notes the reply carrying it
func claimDraw(guild, user string) (publish func(link string), done func(), err error) {
	key := guild + "/" + user
	drawMu.Lock()
	defer drawMu.Unlock()
	if link, ok := draws[key]; ok {
		return nil, nil, &drawingError{link: link}
	}
	draws[key] = ""
	publish = func(link string) {
		drawMu.Lock()
		draws[key] = link
		drawMu.Unlock()
	}
	done = func() {
		drawMu.Lock()
		delete(draws, key)
		drawMu.Unlock()
	}
	return publish, done, nil
}

// draws the card at path (see trendPath) unless it's already on disk. onDraw
// goes first and returns a link to the reply that will carry it; if one of
// the player's cards is already being drawn, nothing is and the error is a
// *drawingError. the period's older cards and links are removed
func cachedTrend(s *discordgo.Session, bcp *types.BotCommandPackage, path, guild, guildName, user, period string, games []scores.PlayerScore, onDraw func() (string, error)) error {
	dir, name := filepath.Dir(path), filepath.Base(path)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	publish, done, err := claimDraw(guild, user)
	if err != nil {
		return err
	}
	defer done()
	link, err := onDraw()
	if err != nil {
		return err
	}
	publish(link)
	if err := trendImage(s, bcp, dir, name, guild, guildName, user, period, games); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "trend_"+period+"_*"))
	for _, f := range old {
		if f != path {
			os.Remove(f) //nolint // a leftover is only disk space
		}
	}
	return nil
}

// draws their games (at least one) over the period as dir/name
func trendImage(s *discordgo.Session, bcp *types.BotCommandPackage, dir, name, guild, guildName, user, period string, games []scores.PlayerScore) error {
	req := dailygen.TrendRequest{
		Guild:     guild,
		User:      user,
		Name:      name,
		Title:     displayName(s, guild, user),
		GuildName: guildName,
		Points:    make([]dailygen.TrendPoint, len(games)),
	}
	for j, g := range games {
		req.Points[j] = dailygen.TrendPoint{Day: g.Day, Score: g.Score}
	}
	window, label := maWindow(games[len(games)-1].Day - games[0].Day + 1)
	req.MA, req.Window = movingAverage(games, window), label
	rows, _, err := bcp.Leaderboard.Board(guild, leaderboard.BEST, period)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.UserID == user {
			req.Rank = &r.Rank
			break
		}
	}

	// no icon on the guild: just their avatar
	images, _, _, err := daily.SaveAvatars(s, dir, &discordgo.Guild{ID: guild}, []string{user})
	if err != nil {
		return fmt.Errorf("saving avatar: %v", err)
	}
	defer os.RemoveAll(images) //nolint // only needed while drawing
	req.Images = filepath.Base(images)
	ctx, cancel := context.WithTimeout(context.Background(), TREND_TIMEOUT)
	defer cancel()
	_, err = dailygen.RenderTrend(ctx, req)
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("the renderer took longer than %v", TREND_TIMEOUT)
	}
	return err
}

func imageEmbed(description, url string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{Description: description, Color: theme.ACCENT}
	if url != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: url}
	}
	return embed
}

// adds the trend card under /topu's (already sent) embed: a placeholder while
// it draws, then the card, or why there isn't one. nothing for someone left
// off the boards or with no games in the period. a card already posted for
// the same games is linked again if discord still has it, else one already
// drawn is uploaded again, with no placeholder either way
func attachTrend(s *discordgo.Session, bcp *types.BotCommandPackage, i *discordgo.InteractionCreate, ranks *discordgo.MessageEmbed, guildName, user, period string) error {
	guild := i.GuildID
	if banned, err := bcp.Leaderboard.IsBlacklisted(guild, user); err != nil || banned {
		return err
	}
	entries, err := bcp.Scores.Guild(guild, nil, leaderboard.MinDay(period, krillion.Today()))
	if err != nil {
		return err
	}
	games := history(entries, user)
	if len(games) == 0 {
		return nil
	}
	noPings := &discordgo.MessageAllowedMentions{}
	send := func(image *discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error) {
		embeds := []*discordgo.MessageEmbed{ranks, image}
		return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds, Files: files, AllowedMentions: noPings})
	}
	fail := func(reason error) error {
		_, err := send(imageEmbed("-# couldn't produce image: "+reason.Error(), ""), nil)
		return errors.Join(fmt.Errorf("trend image: %v", reason), err)
	}

	path := trendPath(bcp, guild, user, period, games)
	if url, at, ok := daily.ReadLink(trendLink(path)); ok {
		_, err := send(imageEmbed(fmt.Sprintf("-# Image reused from <t:%d:R>", at.Unix()), url), nil)
		return err
	}
	placed := false // the placeholder went out, so a failure has a message to edit
	err = cachedTrend(s, bcp, path, guild, guildName, user, period, games, func() (string, error) {
		msg, err := send(imageEmbed("-# generating image...", ""), nil)
		if err != nil {
			return "", err
		}
		placed = true
		return daily.JumpLink(guild, msg), nil
	})
	var drawing *drawingError
	if errors.As(err, &drawing) {
		// like /forcedaily: point at the draw underway instead of starting another
		note := "-# Already generating"
		if drawing.link != "" {
			note += ": " + drawing.link
		}
		_, err := send(imageEmbed(note, ""), nil)
		return err
	}
	if err != nil {
		if !placed {
			return err
		}
		return fail(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(err)
	}
	defer f.Close() //nolint
	name := filepath.Base(path)
	final, err := send(imageEmbed("", "attachment://"+name), []*discordgo.File{{Name: name, ContentType: "image/webp", Reader: f}})
	if err != nil {
		return fail(err)
	}
	url := daily.PostedURL(final)
	if url == "" {
		return errors.New("trend image: posted, but discord returned no url to reuse")
	}
	if err := daily.WriteLink(trendLink(path), url); err != nil {
		return fmt.Errorf("trend image: saving its link: %v", err)
	}
	return nil
}
