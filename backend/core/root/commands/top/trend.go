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

// where a player's card for these games lives
func trendPath(bcp *types.BotCommandPackage, guild, user, period string, games []scores.PlayerScore) string {
	return filepath.Join(bcp.DataDir, dailygen.DIR, guild, "users", user, trendName(period, games))
}

// next to a card: the discord link it was last posted at
func trendLink(path string) string {
	return strings.TrimSuffix(path, ".webp") + ".link"
}

// one draw per card: whoever finds it missing draws it while the rest wait
var trendLocks sync.Map // card path -> *sync.Mutex

func lockTrend(path string) (unlock func()) {
	mu, _ := trendLocks.LoadOrStore(path, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	return mu.(*sync.Mutex).Unlock
}

// draws the card at path (see trendPath) unless it's already on disk,
// calling onDraw first. the period's older cards and links are removed
func cachedTrend(s *discordgo.Session, bcp *types.BotCommandPackage, path, guild, guildName, user, period string, games []scores.PlayerScore, onDraw func() error) error {
	dir, name := filepath.Dir(path), filepath.Base(path)
	defer lockTrend(path)()
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := onDraw(); err != nil {
		return err
	}
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
	err = cachedTrend(s, bcp, path, guild, guildName, user, period, games, func() error {
		_, err := send(imageEmbed("-# generating image...", ""), nil)
		placed = err == nil
		return err
	})
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
