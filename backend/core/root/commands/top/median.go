package top

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"krill-agg/root/commands/daily"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/confirm"
	"krill-agg/root/managers/distributions"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// /topg: a guild's median score day by day against every krillion player's,
// as one card (dailygen's /median). only finished days are drawn: today's
// medians still move as people play, and krillion's histogram for a day is
// only saved by its report. cards are named by what they show and cached like
// /topu's

// the claim's (see claimDraw) and the card folder's name for a guild's own
// cards. player ids are all digits, so it never meets one
const SERVER = "server"

// /topg looks back a month or more: a week of daily medians is too few to average
var MEDIAN_PERIODS = []string{leaderboard.MONTH, leaderboard.MONTHS3, leaderboard.MONTHS6, leaderboard.YTD, leaderboard.ALLTIME}

// how long the bot waits on dailygen for the card
const MEDIAN_TIMEOUT = TREND_TIMEOUT

// the middle of sorted scores (at least one)
func medianOf(sorted []int) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2
}

// the score half of krillion's histogram is under (see krillionapi.Quantile).
// ok=false with no players
func distMedian(dist []float64) (float64, bool) {
	return krillionapi.Quantile(dist, 0.5)
}

// what /topg draws: both series over days first..last
type medianSpan struct {
	first, last int
	guild       dailygen.MedianSeries
	global      dailygen.MedianSeries
	players     int
	window      string   // the moving averages' label
	better      *float64 // percent of everyone's games under the guild's median; nil without histograms
}

// the guild's games (and krillion's histograms) from minDay through last, by
// day. first is the guild's first game in that span; ok=false with none
func buildSpan(entries []scores.Entry, dist func(day int) (krillionapi.Stats, bool), minDay, last int) (span medianSpan, ok bool) {
	byDay := map[int][]int{}
	players := map[string]bool{}
	var all []int
	for _, e := range entries {
		if d := e.Score.Day; d >= minDay && d <= last {
			byDay[d] = append(byDay[d], e.Score.Score)
			players[e.UserID] = true
			all = append(all, e.Score.Score)
		}
	}
	if len(all) == 0 {
		return medianSpan{}, false
	}
	days := make([]int, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	slices.Sort(days)
	span = medianSpan{first: days[0], last: last, players: len(players)}

	for _, d := range days {
		slices.Sort(byDay[d])
		span.guild.Points = append(span.guild.Points, dailygen.TrendAverage{Day: d, Value: medianOf(byDay[d])})
	}
	slices.Sort(all)
	m := medianOf(all)
	span.guild.Median, span.guild.Dives = &m, len(all)

	var summed []float64 // every day's histogram added up, for the span's median
	for d := span.first; d <= last; d++ {
		s, ok := dist(d)
		if !ok || !distributions.HasDist(s) {
			continue
		}
		dm, ok := distMedian(s.Dist)
		if !ok {
			continue
		}
		span.global.Points = append(span.global.Points, dailygen.TrendAverage{Day: d, Value: dm})
		span.global.Dives += s.Sample
		if len(summed) < len(s.Dist) {
			summed = append(summed, make([]float64, len(s.Dist)-len(summed))...)
		}
		for b, count := range s.Dist {
			summed[b] += count
		}
	}
	if gm, ok := distMedian(summed); ok {
		span.global.Median = &gm
		total := 0.0
		for _, count := range summed {
			total += count
		}
		// the same estimate as a day's percentiles, over the whole span
		better := krillionapi.Stats{Sample: int(math.Round(total)), Dist: summed}.Percentile(m)
		span.better = &better
	}

	window, label := maWindow(last - span.first + 1)
	span.window = label
	span.guild.MA = seriesAverage(span.guild.Points, window)
	span.global.MA = seriesAverage(span.global.Points, window)
	return span, true
}

// a short digest of everything the card shows (but the guild's name), so a
// dropped score or a histogram saved late makes a new card, not a stale one
func (span medianSpan) digest() string {
	raw, _ := json.Marshal(struct {
		G, W    dailygen.MedianSeries
		Players int
	}{span.guild, span.global, span.players})
	h := fnv.New32a()
	h.Write(raw) //nolint // never fails
	return fmt.Sprintf("%08x", h.Sum32())
}

// a guild's own cards
func medianDir(bcp *types.BotCommandPackage, guild string) string {
	return filepath.Join(bcp.DataDir, dailygen.DIR, guild, SERVER)
}

// where the card for this span lives: "topg_month_100-130_1a2b3c4d.webp"
func medianPath(bcp *types.BotCommandPackage, guild, period string, span medianSpan) string {
	name := fmt.Sprintf("topg_%s_%d-%d_%s.webp", period, span.first, span.last, span.digest())
	return filepath.Join(medianDir(bcp, guild), name)
}

// cached first; the api only when the state is missing it. bare (no icon)
// when discord won't say
func guildOf(s *discordgo.Session, guild string) *discordgo.Guild {
	if g, err := s.State.Guild(guild); err == nil && g.Name != "" {
		return g
	}
	if g, err := s.Guild(guild); err == nil {
		return g
	}
	return &discordgo.Guild{ID: guild}
}

// draws the span as path, the guild's icon riding its line
func medianImage(s *discordgo.Session, path, guild, title string, span medianSpan) error {
	images, icon, _, err := daily.SaveAvatars(s, filepath.Dir(path), guildOf(s, guild), nil)
	if err != nil {
		return fmt.Errorf("saving the server icon: %v", err)
	}
	defer os.RemoveAll(images) //nolint // only needed while drawing
	ctx, cancel := context.WithTimeout(context.Background(), MEDIAN_TIMEOUT)
	defer cancel()
	_, err = dailygen.RenderMedian(ctx, dailygen.MedianRequest{
		Guild:      guild,
		Name:       filepath.Base(path),
		Title:      title,
		Images:     filepath.Base(images),
		Icon:       icon,
		First:      span.first,
		Last:       span.last,
		GuildS:     span.guild,
		GlobalS:    span.global,
		Window:     span.window,
		Percentile: span.better,
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("the renderer took longer than %v", MEDIAN_TIMEOUT)
	}
	return err
}

// draws the card at path unless it's already on disk, under the guild's one
// claim (any period). onDraw goes first and returns a link to the reply that
// will carry it; if the guild's card is already being drawn, nothing is and
// the error is a *drawingError. the period's older cards and links go
func cachedMedian(s *discordgo.Session, path, guild, title, period string, span medianSpan, onDraw func() (string, error)) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	publish, done, err := claimDraw(guild, SERVER)
	if err != nil {
		return err
	}
	defer done()
	link, err := onDraw()
	if err != nil {
		return err
	}
	publish(link)
	if err := medianImage(s, path, guild, title, span); err != nil {
		return err
	}
	keep := strings.TrimSuffix(path, ".webp")
	old, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "topg_"+period+"_*"))
	for _, f := range old {
		if strings.TrimSuffix(f, filepath.Ext(f)) != keep {
			os.Remove(f) //nolint // a leftover is only disk space
		}
	}
	return nil
}

// removes a guild's cached /topg cards and links, so the next /topg draws
// fresh. busy (nothing removed) while one is being drawn
func clearMedians(bcp *types.BotCommandPackage, guild string) (cards int, busy bool, err error) {
	_, done, err := claimDraw(guild, SERVER)
	if err != nil {
		return 0, true, nil
	}
	defer done()
	dir := medianDir(bcp, guild)
	found, _ := filepath.Glob(filepath.Join(dir, "topg_*.webp"))
	if err := os.RemoveAll(dir); err != nil {
		return 0, false, err
	}
	return len(found), false, nil
}

func commas(n int) string {
	s := strconv.Itoa(n)
	for j := len(s) - 3; j > 0; j -= 3 {
		s = s[:j] + "," + s[j:]
	}
	return s
}

func dives(n int) string {
	return commas(n) + " dive" + plural(n)
}

// the medians in words, over the card
func medianSummary(span medianSpan, name string) string {
	lines := []string{fmt.Sprintf("**%s** · median `%.0f` over %s by %d player%s",
		name, *span.guild.Median, dives(span.guild.Dives), span.players, plural(span.players))}
	if span.global.Median != nil {
		lines = append(lines, fmt.Sprintf("**All players** · median `%.0f` over %s", *span.global.Median, dives(span.global.Dives)))
	} else {
		lines = append(lines, "**All players** · no krillion histograms saved for these days")
	}
	lines = append(lines, fmt.Sprintf("-# Krillion #%d–#%d. Today's joins once it's over.", span.first, span.last))
	return strings.Join(lines, "\n")
}

// /topg: the summary right away, then the card under it (a placeholder while
// it draws), or why there isn't one. a card already posted for the same span
// is linked again if discord still has it, else one already drawn is
// uploaded again, with no placeholder either way
func openMedians(s *discordgo.Session, bcp *types.BotCommandPackage, i *discordgo.InteractionCreate, period string) error {
	guild := i.GuildID
	name := guildName(s, guild)
	exclude, err := bcp.Leaderboard.Blacklist(guild)
	if err != nil {
		return err
	}
	today := krillion.Today()
	minDay := leaderboard.MinDay(period, today)
	entries, err := bcp.Scores.Guild(guild, exclude, minDay)
	if err != nil {
		return err
	}
	span, ok := buildSpan(entries, bcp.Distributions.Get, minDay, today-1)
	summary := imageEmbed("No finished days to chart yet!", "")
	summary.Title = boardTitle("Median Scores", period, name)
	if ok {
		summary.Description = medianSummary(span, name)
	}
	noPings := &discordgo.MessageAllowedMentions{}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{summary}, AllowedMentions: noPings},
	}); err != nil || !ok {
		return err
	}

	send := func(image *discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error) {
		embeds := []*discordgo.MessageEmbed{summary, image}
		return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds, Files: files, AllowedMentions: noPings})
	}
	fail := func(reason error) error {
		_, err := send(imageEmbed("-# couldn't produce image: "+reason.Error(), ""), nil)
		return errors.Join(fmt.Errorf("median image: %v", reason), err)
	}

	path := medianPath(bcp, guild, period, span)
	if url, at, ok := daily.ReadLink(trendLink(path)); ok {
		_, err := send(imageEmbed(fmt.Sprintf("-# Image reused from <t:%d:R>", at.Unix()), url), nil)
		return err
	}
	placed := false // the placeholder went out, so a failure has a message to edit
	err = cachedMedian(s, path, guild, name, period, span, func() (string, error) {
		msg, err := send(imageEmbed("-# generating image...", ""), nil)
		if err != nil {
			return "", err
		}
		placed = true
		return daily.JumpLink(guild, msg), nil
	})
	var drawing *drawingError
	if errors.As(err, &drawing) {
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
	base := filepath.Base(path)
	final, err := send(imageEmbed("", "attachment://"+base), []*discordgo.File{{Name: base, ContentType: "image/webp", Reader: f}})
	if err != nil {
		return fail(err)
	}
	// the summary is embed 0, the card 1
	var url string
	if len(final.Embeds) > 1 && final.Embeds[1].Image != nil && strings.HasPrefix(final.Embeds[1].Image.URL, "http") {
		url = final.Embeds[1].Image.URL
	} else if len(final.Attachments) > 0 {
		url = final.Attachments[0].URL
	}
	if url == "" {
		return errors.New("median image: posted, but discord returned no url to reuse")
	}
	if err := daily.WriteLink(trendLink(path), url); err != nil {
		return fmt.Errorf("median image: saving its link: %v", err)
	}
	return nil
}

func registerMedians(bcp *types.BotCommandPackage) {
	periods := make([]*discordgo.ApplicationCommandOptionChoice, len(MEDIAN_PERIODS))
	for j, p := range MEDIAN_PERIODS {
		periods[j] = &discordgo.ApplicationCommandOptionChoice{Name: leaderboard.PERIOD_LABELS[p], Value: p}
	}
	(*bcp.Handles)["topg"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "topg",
			Description: "this server's median score against every krillion player's",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "since",
				Description: "how far back to count (default: all-time)",
				Choices:     periods,
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			return openMedians(s, bcp, i, option(i, "since", leaderboard.ALLTIME))
		},
	}
	(*bcp.Handles)["mg-cleartopg"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "mg-cleartopg",
			Description: "delete this server's cached /topg cards so they're drawn fresh",
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			cards, busy, err := clearMedians(bcp, i.GuildID)
			if err != nil {
				return err
			}
			text := fmt.Sprintf("Cleared %d cached /topg card%s.", cards, plural(cards))
			if busy {
				text = "Nothing was cleared: /topg cards are being drawn right now."
			}
			return confirm.Reply(s, i, text)
		},
	}
}
