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

// their games (at least one) over the period as a trend card; its path on disk
func trendImage(s *discordgo.Session, bcp *types.BotCommandPackage, guild, guildName, user, period string, games []scores.PlayerScore) (string, error) {
	req := dailygen.TrendRequest{
		Guild:     guild,
		User:      user,
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
		return "", err
	}
	for _, r := range rows {
		if r.UserID == user {
			req.Rank = &r.Rank
			break
		}
	}

	root := filepath.Join(bcp.DataDir, dailygen.DIR)
	// no icon on the guild: just their avatar
	images, _, _, err := daily.SaveAvatars(s, filepath.Join(root, guild, "users", user), &discordgo.Guild{ID: guild}, []string{user})
	if err != nil {
		return "", fmt.Errorf("saving avatar: %v", err)
	}
	defer os.RemoveAll(images) //nolint // only needed while drawing
	req.Images = filepath.Base(images)
	ctx, cancel := context.WithTimeout(context.Background(), TREND_TIMEOUT)
	defer cancel()
	path, err := dailygen.RenderTrend(ctx, req)
	if errors.Is(err, context.DeadlineExceeded) {
		return "", fmt.Errorf("the renderer took longer than %v", TREND_TIMEOUT)
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(root, path), nil
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
// off the boards or with no games in the period. always drawn anew
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
	send := func(image *discordgo.MessageEmbed, files []*discordgo.File) error {
		embeds := []*discordgo.MessageEmbed{ranks, image}
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds, Files: files, AllowedMentions: noPings})
		return err
	}
	fail := func(reason error) error {
		return errors.Join(fmt.Errorf("trend image: %v", reason), send(imageEmbed("-# couldn't produce image: "+reason.Error(), ""), nil))
	}

	if err := send(imageEmbed("-# generating image...", ""), nil); err != nil {
		return err
	}
	path, err := trendImage(s, bcp, guild, guildName, user, period, games)
	if err != nil {
		return fail(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(err)
	}
	defer f.Close() //nolint
	name := filepath.Base(path)
	if err := send(imageEmbed("", "attachment://"+name), []*discordgo.File{{Name: name, ContentType: "image/webp", Reader: f}}); err != nil {
		return fail(err)
	}
	return nil
}
