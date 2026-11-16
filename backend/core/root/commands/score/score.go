package score

import (
	"fmt"
	"krill-agg/root/commands/stats"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"
	"strconv"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

var metadata = types.CommandMetadata{
	MinTrustLevel: authorize.DEFAULT,
}

func edit(s *discordgo.Session, i *discordgo.InteractionCreate, edit *discordgo.WebhookEdit) error {
	edit.AllowedMentions = &discordgo.MessageAllowedMentions{}
	_, err := s.InteractionResponseEdit(i.Interaction, edit)
	return err
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	(*bcp.Handles)["score"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "score",
			Description: "show a krillion result and where it ranks",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "whose result (default: you)",
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "day",
					Description: "krillion # (default: today)",
				},
			},
		},
		Metadata: metadata,
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			// the api calls can outlast discord's 3s window
			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			}); err != nil {
				return err
			}

			user := i.Member.User
			today := krillion.Today()
			day := today
			for _, opt := range i.ApplicationCommandData().Options {
				switch opt.Name {
				case "user":
					if u := opt.UserValue(s); u != nil {
						user = u
					}
				case "day":
					day = int(opt.IntValue())
				}
			}

			p, ok, err := bcp.Scores.Get(i.GuildID, day, user.ID)
			if err != nil {
				return err
			}
			if !ok {
				content := fmt.Sprintf("No result for <@%s> on Krillion #%d.", user.ID, day)
				return edit(s, i, &discordgo.WebhookEdit{Content: &content})
			}

			// krillion only ranks today's puzzle
			rank := stats.PAST_NOTE
			if day == today {
				dist, err := krillionapi.Complete(krillion.DateOf(day), p.Score)
				if err != nil {
					rank = "-# Couldn't reach krillion for a percentile."
				} else {
					rank = fmt.Sprintf("Better than `%s%%` of players", strconv.FormatFloat(dist.BetterThan, 'f', -1, 64))
				}
			}

			embeds := []*discordgo.MessageEmbed{{
				Author:      &discordgo.MessageEmbedAuthor{Name: user.DisplayName(), IconURL: user.AvatarURL("")},
				Description: p.Message() + "\n\n" + rank,
			}}
			return edit(s, i, &discordgo.WebhookEdit{Embeds: &embeds})
		},
	}
}
