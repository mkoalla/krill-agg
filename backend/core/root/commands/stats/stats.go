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

func description(bcp *types.BotCommandPackage, guild string, day int) (string, error) {
	entries, err := bcp.Scores.Day(guild, day)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "No players yet!", nil
	}
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = fmt.Sprintf("<@%s> - %s", e.UserID, e.Score.Format())
	}
	return strings.Join(lines, "\n"), nil
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
			desc, err := description(bcp, i.GuildID, day)
			if err != nil {
				return err
			}

			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					// discord always renders content above embeds
					Content: FOOTER,
					Embeds: []*discordgo.MessageEmbed{
						{
							Title:       fmt.Sprintf("Krillion #%d 🦐", day),
							Description: desc,
						},
					},
					AllowedMentions: &discordgo.MessageAllowedMentions{},
				},
			})
		},
	}
}
