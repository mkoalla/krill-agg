package top

import (
	"fmt"
	"krill-agg/root/info/theme"
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

// add/remove answer in plain text; show answers with an embed, where the
// mentions just show profiles
func blacklistReply(bcp *types.BotCommandPackage, i *discordgo.InteractionCreate) (*discordgo.InteractionResponseData, error) {
	content, embed, err := blacklistAnswer(bcp, i)
	if err != nil {
		return nil, err
	}
	data := &discordgo.InteractionResponseData{Content: content, AllowedMentions: &discordgo.MessageAllowedMentions{}}
	if embed != nil {
		data.Embeds = []*discordgo.MessageEmbed{embed}
	}
	return data, nil
}

func blacklistAnswer(bcp *types.BotCommandPackage, i *discordgo.InteractionCreate) (string, *discordgo.MessageEmbed, error) {
	guild := i.GuildID
	sub, user := blacklistArgs(i)
	switch sub {
	case "add", "remove":
		changed, err := bcp.Leaderboard.SetBlacklisted(guild, user, sub == "add")
		if err != nil {
			return "", nil, err
		}
		switch {
		case sub == "add" && changed:
			return fmt.Sprintf("<@%s> is now left off this server's leaderboards.", user), nil, nil
		case sub == "add":
			return fmt.Sprintf("<@%s> is already blacklisted.", user), nil, nil
		case changed:
			return fmt.Sprintf("<@%s> is back on this server's leaderboards.", user), nil, nil
		default:
			return fmt.Sprintf("<@%s> wasn't blacklisted.", user), nil, nil
		}
	default:
		ids, err := bcp.Leaderboard.Blacklist(guild)
		if err != nil {
			return "", nil, err
		}
		return "", blacklistEmbed(ids), nil
	}
}

func blacklistEmbed(ids []string) *discordgo.MessageEmbed {
	body := "Nobody is blacklisted."
	if len(ids) > 0 {
		lines := make([]string, len(ids))
		for j, id := range ids {
			lines[j] = fmt.Sprintf("<@%s>", id)
		}
		body = strings.Join(lines, "\n")
	}
	return &discordgo.MessageEmbed{Title: "Left off the leaderboards", Description: body, Color: theme.ACCENT}
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
			data, err := blacklistReply(bcp, i)
			if err != nil {
				return err
			}
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: data,
			})
		},
	}
}
