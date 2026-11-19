package score

import (
	"fmt"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/info/theme"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/distributions"
	"krill-agg/root/types"

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
					Description: krillion.DAY_OPTION + " (default: today)",
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
					day = krillion.Resolve(int(opt.IntValue()))
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

			// from the histogram rather than krillion's betterThan, which is
			// rounded to a whole percent (95.5% reads as 96). krillion only
			// serves today's; older days use the one saved for them
			var rank string
			if day == today {
				dist, err := krillionapi.Complete(krillion.DateOf(day), p.Score)
				if err != nil {
					rank = "-# Couldn't reach krillion for a percentile."
				} else {
					rank = fmt.Sprintf("Better than `%s%%` of players", dist.PercentileText(float64(p.Score)))
				}
			} else if dist, ok := bcp.Distributions.Get(day); ok && distributions.HasDist(dist) {
				rank = fmt.Sprintf("Better than `%s%%` of players that day", dist.PercentileText(float64(p.Score)))
			} else {
				rank = "-# Krillion has no score distribution saved for that day."
			}

			embeds := []*discordgo.MessageEmbed{{
				Author:      &discordgo.MessageEmbedAuthor{Name: user.DisplayName(), IconURL: user.AvatarURL("")},
				Description: p.Message() + "\n\n" + rank,
				Color:       theme.ACCENT,
			}}
			return edit(s, i, &discordgo.WebhookEdit{Embeds: &embeds})
		},
	}
}
