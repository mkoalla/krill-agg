package channels

import (
	"fmt"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

type session = *discordgo.Session
type intxn = *discordgo.InteractionCreate
type bcpackage = *types.BotCommandPackage

// every command acts on the guild it was run in; there's no way to target another
type action = func(s session, i intxn, bcp bcpackage, all bool) (summary string, showAll bool, err error)

func allOption(description string) []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionBoolean,
			Name:        "all",
			Description: description,
		},
	}
}

func getAll(i intxn) bool {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "all" {
			return opt.BoolValue()
		}
	}
	return false
}

// text channels in the guild, in sidebar order
func textChannels(s session, guild string) ([]*discordgo.Channel, error) {
	channels, err := s.GuildChannels(guild)
	if err != nil {
		return nil, err
	}
	channels = slices.DeleteFunc(channels, func(c *discordgo.Channel) bool {
		return c.Type != discordgo.ChannelTypeGuildText
	})
	slices.SortFunc(channels, func(a, b *discordgo.Channel) int { return a.Position - b.Position })
	return channels, nil
}

// tracked channels, or every text channel with the tracked ones marked
func listing(s session, i intxn, bcp bcpackage, showAll bool) (string, error) {
	if !showAll {
		tracked := bcp.Tracking.Channels(i.GuildID)
		if len(tracked) == 0 {
			return "No channels tracked yet!", nil
		}
		lines := make([]string, len(tracked))
		for j, c := range tracked {
			lines[j] = fmt.Sprintf("<#%s>", c)
		}
		return strings.Join(lines, "\n"), nil
	}

	channels, err := textChannels(s, i.GuildID)
	if err != nil {
		return "", err
	}
	if len(channels) == 0 {
		return "No text channels found.", nil
	}
	lines := make([]string, len(channels))
	for j, c := range channels {
		mark := "➖"
		if bcp.Tracking.IsTracked(i.GuildID, c.ID) {
			mark = "✅"
		}
		lines[j] = fmt.Sprintf("%s <#%s>", mark, c.ID)
	}
	return strings.Join(lines, "\n"), nil
}

func show(s session, i intxn, bcp bcpackage, all bool) (string, bool, error) {
	return "", all, nil
}

func track(s session, i intxn, bcp bcpackage, all bool) (string, bool, error) {
	if !all {
		added, err := bcp.Tracking.Track(i.GuildID, i.ChannelID)
		if err != nil {
			return "", false, err
		}
		if added == 0 {
			return fmt.Sprintf("<#%s> is already tracked.", i.ChannelID), false, nil
		}
		return fmt.Sprintf("Now tracking <#%s>.", i.ChannelID), false, nil
	}
	channels, err := textChannels(s, i.GuildID)
	if err != nil {
		return "", false, err
	}
	ids := make([]string, len(channels))
	for j, c := range channels {
		ids[j] = c.ID
	}
	added, err := bcp.Tracking.Track(i.GuildID, ids...)
	if err != nil {
		return "", false, err
	}
	return fmt.Sprintf("Now tracking %d more channel(s).", added), false, nil
}

func untrack(s session, i intxn, bcp bcpackage, all bool) (string, bool, error) {
	if all {
		removed, err := bcp.Tracking.UntrackAll(i.GuildID)
		if err != nil {
			return "", false, err
		}
		return fmt.Sprintf("Stopped tracking %d channel(s).", removed), false, nil
	}
	removed, err := bcp.Tracking.Untrack(i.GuildID, i.ChannelID)
	if err != nil {
		return "", false, err
	}
	if removed == 0 {
		return fmt.Sprintf("<#%s> wasn't tracked.", i.ChannelID), false, nil
	}
	return fmt.Sprintf("Stopped tracking <#%s>.", i.ChannelID), false, nil
}

func respond(do action, bcp bcpackage) EventCallback {
	return func(s session, i intxn) error {
		summary, showAll, err := do(s, i, bcp, getAll(i))
		if err != nil {
			return err
		}
		list, err := listing(s, i, bcp, showAll)
		if err != nil {
			return err
		}
		title := "Tracked channels"
		if showAll {
			title = "Text channels"
		}
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content:         summary,
				Embeds:          []*discordgo.MessageEmbed{{Title: title, Description: list}},
				AllowedMentions: &discordgo.MessageAllowedMentions{},
			},
		})
	}
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	(*bcp.Handles)["show"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "show",
			Description: "list channels where krillion results are read",
			Options:     allOption("list every text channel, marking tracked ones"),
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: respond(show, bcp),
	}
	(*bcp.Handles)["track"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "track",
			Description: "read krillion results in this channel",
			Options:     allOption("track every text channel in this server"),
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: respond(track, bcp),
	}
	(*bcp.Handles)["untrack"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "untrack",
			Description: "stop reading krillion results in this channel",
			Options:     allOption("untrack every channel in this server"),
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: respond(untrack, bcp),
	}
}
