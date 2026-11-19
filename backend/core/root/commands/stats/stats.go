package stats

import (
	"fmt"
	"krill-agg/root/info/theme"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/rivo/uniseg"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

const (
	// %d is the next reset; discord renders the stamp as "in 5 hours"
	FOOTER = "-# Percentiles and playcounts at daily reset <t:%d:R>!"
	// krillion only ranks today's puzzle, so anything older has no percentiles
	PAST_NOTE = "-# Percentiles are only available for today's puzzle."
)

// the line under a board: what's coming for today, why there's nothing for older days
func Footer(day int) string {
	today := krillion.Today()
	if day < today {
		return PAST_NOTE
	}
	return fmt.Sprintf(FOOTER, krillion.StartOf(today+1).Unix())
}

var metadata = types.CommandMetadata{
	MinTrustLevel: authorize.DEFAULT,
}

// "`🦑🦑🦑🐟🦑🦑🏮` - `415` - <@uid>". the code spans force monospace so the
// columns line up; the mention stays outside so it still renders
func line(uid string, p krillion.PlayerScore) string {
	return fmt.Sprintf("`%s` - `%3d` - <@%s>", padGraphemes(p.Emojis(), krillion.NumChoices), p.Score, uid)
}

// left-aligns to n characters as people see them; fmt's %-7s counts runes,
// and one emoji can be several
func padGraphemes(s string, n int) string {
	if pad := n - uniseg.GraphemeClusterCount(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

const NO_PLAYERS = "No players yet!"

// the day's scoreboard; entries come sorted from scores.Day. shared with /forcedaily
func Board(day int, entries []scores.Entry) *discordgo.MessageEmbed {
	desc := NO_PLAYERS
	if len(entries) > 0 {
		lines := make([]string, len(entries))
		for i, e := range entries {
			lines[i] = line(e.UserID, e.Score)
		}
		desc = strings.Join(lines, "\n")
	}
	return &discordgo.MessageEmbed{Title: krillion.Header(day), Description: desc, Color: theme.ACCENT}
}

// a bold label rather than the embed's title, to match the report's sections
const QUESTIONS_HEADING = "**Today's Questions**"

// "1. ||question||" for each, spoilered so nobody sees them by accident
func QuestionsEmbed(qs []string) *discordgo.MessageEmbed {
	lines := make([]string, len(qs))
	for j, q := range qs {
		lines[j] = fmt.Sprintf("%d. ||%s||", j+1, q)
	}
	return &discordgo.MessageEmbed{Description: QUESTIONS_HEADING + "\n" + strings.Join(lines, "\n"), Color: theme.ACCENT}
}

// the day's saved questions as an embed, or nil if they were never fetched
// (only daily reports fetch them; krillion only shares today's)
func Questions(bcp *types.BotCommandPackage, day int) *discordgo.MessageEmbed {
	qs, ok := bcp.Questions.Get(day)
	if !ok {
		return nil
	}
	return QuestionsEmbed(qs)
}

// answers /stats for a finished day with that day's full report (the daily
// post minus the winners). set by the daily package, which owns reports and
// cards (and imports this one, so it can't be called directly)
var PastReport func(s *discordgo.Session, i *discordgo.InteractionCreate, bcp *types.BotCommandPackage, day int, entries []scores.Entry) error

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	(*bcp.Handles)["stats"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "stats",
			Description: "show krillion standings for a day",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "day",
					Description: krillion.DAY_OPTION + " (default: today)",
				},
			},
		},
		Metadata: metadata,
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			options := i.ApplicationCommandData().Options
			optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
			for _, opt := range options {
				optionMap[opt.Name] = opt
			}
			day := krillion.Today()
			if opt, ok := optionMap["day"]; ok {
				day = krillion.Resolve(int(opt.IntValue()))
			}
			entries, err := bcp.Scores.Day(i.GuildID, day)
			if err != nil {
				return err
			}

			// a day that's over gets its whole report; today's percentiles and
			// card come with the daily post
			if day >= 1 && day < krillion.Today() && PastReport != nil {
				return PastReport(s, i, bcp, day, entries)
			}
			embeds := []*discordgo.MessageEmbed{Board(day, entries)}
			if q := Questions(bcp, day); q != nil {
				embeds = append(embeds, q)
			}
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					// discord always renders content above embeds
					Content:         Footer(day),
					Embeds:          embeds,
					AllowedMentions: &discordgo.MessageAllowedMentions{},
				},
			})
		},
	}
}
