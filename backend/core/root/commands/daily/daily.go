package daily

import (
	"errors"
	"fmt"
	"krill-agg/root/commands/stats"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/distributions"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

var metadata = types.CommandMetadata{
	MinTrustLevel: authorize.DEFAULT,
}

// 3245071 -> "3,245,071"
func commas(n int) string {
	digits := strconv.Itoa(n)
	if n < 0 {
		return "-" + commas(-n)
	}
	var out strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(d)
	}
	return out.String()
}

// "`~53%` - <@uid>", floored so nobody is rounded up past where they are
func percentileLine(uid string, percent float64) string {
	return fmt.Sprintf("`~%02d%%` - <@%s>", int(math.Floor(percent)), uid)
}

const NO_DIST = "Krillion has no score distribution for this day."

// percentiles from one distribution lookup, same order as the board
// (sorted by score, so also by percentile). past days count "that day"
func Percentiles(day int, stats krillionapi.Stats, entries []scores.Entry) string {
	if !distributions.HasDist(stats) {
		return NO_DIST
	}
	when := "today"
	if day < krillion.Today() {
		when = "that day"
	}
	lines := []string{fmt.Sprintf("Players %s: `%s`", when, commas(stats.Sample))}
	if len(entries) > 0 {
		lines = append(lines, "")
	}
	for _, e := range entries {
		lines = append(lines, percentileLine(e.UserID, stats.Percentile(float64(e.Score.Score))))
	}
	return strings.Join(lines, "\n")
}

// the message's own heading, above the embeds: "# Krillion Daily #124 🦐"
func Heading(day int) string {
	return fmt.Sprintf("# Krillion Daily Results #%d 🦐", day)
}

// "Today's Winner: <@uid>", or a dotted list under "Today's Winners:" for a
// tie. empty with no players. entries come sorted best first
func Winners(entries []scores.Entry) string {
	if len(entries) == 0 {
		return ""
	}
	best := entries[0].Score.Score
	var ids []string
	for _, e := range entries {
		if e.Score.Score != best {
			break
		}
		ids = append(ids, e.UserID)
	}
	if len(ids) == 1 {
		return fmt.Sprintf("Today's Winner: <@%s>", ids[0])
	}
	lines := []string{"Today's Winners:"}
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf("- <@%s>", id))
	}
	return strings.Join(lines, "\n")
}

// the text above the embeds: the heading, then who won if asked
func Content(snap *Snapshot, winners bool) string {
	if w := Winners(snap.Entries); winners && w != "" {
		return Heading(snap.Day) + "\n" + w
	}
	return Heading(snap.Day)
}

// the board with the percentiles as a second section of the same embed. the
// day is in the message heading, so the embed is just "Scores"
func Combined(day int, entries []scores.Entry, dist krillionapi.Stats) *discordgo.MessageEmbed {
	embed := stats.Board(day, entries)
	// an embed has one title, so both sections get bold labels instead (a
	// ### heading leaves a gap at the top); the day is in the message heading
	embed.Title = ""
	embed.Description = "**Scores**\n" + embed.Description + "\n\n**Percentiles**\n" + Percentiles(day, dist, entries)
	return embed
}

// a daily report is a natural point to bring the leaderboards up to date
func refetchBoards(bcp *types.BotCommandPackage, guild string) error {
	if err := bcp.Leaderboard.Refetch(guild); err != nil {
		return fmt.Errorf("refetching leaderboards: %v", err)
	}
	return nil
}

// everything one report is built from, kept so the image step needn't refetch
type Snapshot struct {
	Guild   string
	Day     int
	Entries []scores.Entry
	Dist    krillionapi.Stats
}

// the day's board and percentiles in one embed (the api only serves today). one api call, everything else is interpolated
func Report(bcp *types.BotCommandPackage, guild string, day int) ([]*discordgo.MessageEmbed, *Snapshot, error) {
	entries, err := bcp.Scores.Day(guild, day)
	if err != nil {
		return nil, nil, err
	}
	dist, err := krillionapi.Complete(krillion.DateOf(day), krillionapi.MIDDLE_SCORE)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching today's distribution: %v", err)
	}
	// kept so this day's card can be drawn again after krillion moves on
	bcp.Distributions.Save(day, dist) //nolint // optional: only a later redraw needs it
	snap := &Snapshot{Guild: guild, Day: day, Entries: entries, Dist: dist}
	embeds := []*discordgo.MessageEmbed{Combined(day, entries, dist)}
	// between the scores and the card (withImage adds that last). fetched and
	// saved here so /stats can show them later; if krillion won't say, any
	// saved copy still shows
	bcp.Questions.Fetch(day) //nolint // optional: a failure only loses the embed
	// today's krillion answers too, while krillion still reveals them (/krill)
	bcp.Krills.Fetch(day) //nolint // optional: /krill fetches today's itself
	if q := stats.Questions(bcp, day); q != nil {
		embeds = append(embeds, q)
	}
	return embeds, snap, nil
}

// /stats for a finished day: that day's daily report, minus the winners.
// the distribution comes from distributions.db (none saved: no percentiles
// and a flat line) and the questions from questions.db. the card is drawn
// once per day, then reused through the link file kept beside it
// (re-uploaded if discord's link has expired); no cooldown
func pastReport(s *discordgo.Session, i *discordgo.InteractionCreate, bcp *types.BotCommandPackage, day int, entries []scores.Entry) error {
	dist, _ := bcp.Distributions.Get(day)
	snap := &Snapshot{Guild: i.GuildID, Day: day, Entries: entries, Dist: dist}
	embeds := []*discordgo.MessageEmbed{Combined(day, entries, dist)}
	if q := stats.Questions(bcp, day); q != nil {
		embeds = append(embeds, q)
	}
	// the board goes out now; the card follows as an edit
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:         Content(snap, false),
			Embeds:          embeds,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	}); err != nil {
		return err
	}
	t := claimPast(filepath.Join(bcp.DataDir, dailygen.DIR, i.GuildID, strconv.Itoa(day)), day)
	_, err := withImage(s, replySender(s, i), bcp, snap, embeds, t, ImageOptions{})
	return err
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	stats.PastReport = pastReport
	(*bcp.Handles)["forcedaily"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "forcedaily",
			Description: "post today's krillion board with percentiles now",
		},
		Metadata: metadata,
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			// the api calls can outlast discord's 3s window
			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			}); err != nil {
				return err
			}
			day := krillion.Today()
			// one render per guild: a call made while one is underway just links to it
			opts := ImageOptions{}
			t, err := claimFor(s, i, day, opts)
			if t == nil || err != nil {
				return err
			}
			defer t.done()
			embeds, snap, err := Report(bcp, i.GuildID, day)
			if err != nil {
				content := "❌ " + err.Error()
				_, errEdit := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
				if errEdit != nil {
					return errEdit
				}
				return err
			}
			// a person asking again soon gets the recent card instead of a re-render
			_, err = withImage(s, replySender(s, i), bcp, snap, embeds, t, opts)
			return errors.Join(err, refetchBoards(bcp, i.GuildID))
		},
	}
}
