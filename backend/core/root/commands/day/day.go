package day

import (
	"fmt"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

var metadata = types.CommandMetadata{
	MinTrustLevel: authorize.DEFAULT,
}

// "Krillion #124\n-# #125 <t:…:R>"; discord renders the relative stamp as "in 5 hours"
func message(today int) string {
	return fmt.Sprintf("Krillion #%d\n-# #%d <t:%d:R>", today, today+1, krillion.StartOf(today+1).Unix())
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	(*bcp.Handles)["day"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "day",
			Description: "which krillion puzzle is live, and when the next one starts",
		},
		Metadata: metadata,
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Content: message(krillion.Today())},
			})
		},
	}
}
