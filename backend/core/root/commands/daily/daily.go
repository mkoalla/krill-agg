package daily

import (
	"fmt"
	"krill-agg/root/commands/stats"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"math"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

var metadata = types.CommandMetadata{
	MinTrustLevel: authorize.MANAGE,
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
func Percentiles(stats krillionapi.Stats, entries []scores.Entry) *discordgo.MessageEmbed {
	lines := []string{fmt.Sprintf("Players today: `%s`", commas(stats.Sample)), ""}
	if len(entries) == 0 {
		lines = append(lines, "No players yet!")
	}
	for _, e := range entries {
		lines = append(lines, percentileLine(e.UserID, stats.Percentile(e.Score.Score)))
	}
	return &discordgo.MessageEmbed{Title: "Percentiles", Description: strings.Join(lines, "\n")}
}

// today's board plus percentiles. one api call, everything else is interpolated
func Report(bcp *types.BotCommandPackage, guild string) ([]*discordgo.MessageEmbed, error) {
	day := krillion.Today()
	entries, err := bcp.Scores.Day(guild, day)
	if err != nil {
		return nil, err
	}
	dist, err := krillionapi.Complete(krillion.DateOf(day), krillionapi.MIDDLE_SCORE)
	if err != nil {
		return nil, fmt.Errorf("fetching today's distribution: %v", err)
	}
	return []*discordgo.MessageEmbed{stats.Board(day, entries), Percentiles(dist, entries)}, nil
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
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
			embeds, err := Report(bcp, i.GuildID)
			if err != nil {
				content := "❌ " + err.Error()
				_, errEdit := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
				if errEdit != nil {
					return errEdit
				}
				return err
			}
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Embeds:          &embeds,
				AllowedMentions: &discordgo.MessageAllowedMentions{},
			})
			return err
		},
	}
}
