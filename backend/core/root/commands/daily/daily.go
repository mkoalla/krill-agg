package daily

import (
	"errors"
	"fmt"
	"krill-agg/root/commands/stats"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
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

// percentiles from one distribution lookup, same order as the board
// (sorted by score, so also by percentile)
func Percentiles(stats krillionapi.Stats, entries []scores.Entry) string {
	lines := []string{fmt.Sprintf("Players today: `%s`", commas(stats.Sample))}
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
	embed.Title = "Scores"
	embed.Description += "\n### Percentiles\n" + Percentiles(dist, entries)
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
	if q := stats.Questions(bcp, day); q != nil {
		embeds = append(embeds, q)
	}
	return embeds, snap, nil
}

// a past day's card for /stats: the board it already sent stays, the card
// joins it as a last embed. drawn once per day, then reused through the
// link file kept beside it (re-uploaded if discord's link has expired); no
// cooldown. krillion's distribution for the day comes from distributions.db
// (none saved: a flat line)
func pastCard(s *discordgo.Session, i *discordgo.InteractionCreate, bcp *types.BotCommandPackage, day int, entries []scores.Entry, embeds []*discordgo.MessageEmbed) error {
	dist, _ := bcp.Distributions.Get(day)
	snap := &Snapshot{Guild: i.GuildID, Day: day, Entries: entries, Dist: dist}
	reply := replySender(s, i)
	// keep /stats's own text above the embeds, not the daily heading
	post := func(_ string, embeds []*discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error) {
		return reply(stats.Footer(day), embeds, files)
	}
	_, err := withImage(s, post, bcp, snap, embeds, claimPast(filepath.Join(bcp.DataDir, dailygen.DIR, i.GuildID, strconv.Itoa(day)), day), ImageOptions{})
	return err
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	stats.PastCard = pastCard
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
