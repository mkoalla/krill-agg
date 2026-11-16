package stats

import (
	"fmt"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

const FOOTER = "-# Percentiles and playcounts at daily reset!"

var metadata = types.CommandMetadata{
	MinTrustLevel: authorize.DEFAULT,
}

func description(bcp *types.BotCommandPackage, guild string, day int) string {
	entries, ok := bcp.Scores.Day(guild, day)
	if !ok {
		return "No players yet!"
	}
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = fmt.Sprintf("<@%s> - %s", e.UserID, e.Score.Format())
	}
	return strings.Join(lines, "\n")
}

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
					Description: "krillion #",
					Required:    true,
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
			day := int(optionMap["day"].IntValue())

			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					// discord always renders content above embeds
					Content: FOOTER,
					Embeds: []*discordgo.MessageEmbed{
						{
							Title:       fmt.Sprintf("Krillion #%d 🦐", day),
							Description: description(bcp, i.GuildID, day),
						},
					},
					AllowedMentions: &discordgo.MessageAllowedMentions{},
				},
			})
		},
	}
}
