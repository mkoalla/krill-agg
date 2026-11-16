package top

import (
	"fmt"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func userOption(description string) []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{{
		Type:        discordgo.ApplicationCommandOptionUser,
		Name:        "user",
		Description: description,
		Required:    true,
	}}
}

// "show", or "add"/"remove" and the chosen user's id
func blacklistArgs(i *discordgo.InteractionCreate) (sub string, user string) {
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return "", ""
	}
	sub = opts[0].Name
	for _, o := range opts[0].Options {
		if o.Name == "user" {
			user = o.Value.(string)
		}
	}
	return sub, user
}

func blacklistReply(bcp *types.BotCommandPackage, i *discordgo.InteractionCreate) (string, error) {
	guild := i.GuildID
	sub, user := blacklistArgs(i)
	switch sub {
	case "add", "remove":
		changed, err := bcp.Leaderboard.SetBlacklisted(guild, user, sub == "add")
		if err != nil {
			return "", err
		}
		switch {
		case sub == "add" && changed:
			return fmt.Sprintf("<@%s> is now left off this server's leaderboards.", user), nil
		case sub == "add":
			return fmt.Sprintf("<@%s> is already blacklisted.", user), nil
		case changed:
			return fmt.Sprintf("<@%s> is back on this server's leaderboards.", user), nil
		default:
			return fmt.Sprintf("<@%s> wasn't blacklisted.", user), nil
		}
	default:
		ids, err := bcp.Leaderboard.Blacklist(guild)
		if err != nil {
			return "", err
		}
		if len(ids) == 0 {
			return "Nobody is blacklisted.", nil
		}
		lines := make([]string, len(ids))
		for j, id := range ids {
			lines[j] = fmt.Sprintf("<@%s>", id)
		}
		return "**Left off the leaderboards:**\n" + strings.Join(lines, "\n"), nil
	}
}

func registerBlacklist(bcp *types.BotCommandPackage) {
	(*bcp.Handles)["blacklist"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "blacklist",
			Description: "leave people off this server's leaderboards",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: "show", Description: "list who's left off"},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: "add", Description: "leave someone off the leaderboards",
					Options: userOption("who to leave off")},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: "remove", Description: "put someone back on the leaderboards",
					Options: userOption("who to put back")},
			},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			content, err := blacklistReply(bcp, i)
			if err != nil {
				return err
			}
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content:         content,
					AllowedMentions: &discordgo.MessageAllowedMentions{},
				},
			})
		},
	}
}
