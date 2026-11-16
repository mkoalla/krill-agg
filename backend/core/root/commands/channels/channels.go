package channels

import (
	"fmt"
	"krill-agg/root/info/theme"
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

const MAIN_MARK = "⭐"

const MAIN_HINT = "Change it with `/mainchan`."

// tracked channels (main one starred), or every text channel with the tracked ones marked
func listing(s session, i intxn, bcp bcpackage, showAll bool) (string, error) {
	main, _ := bcp.Tracking.Main(i.GuildID)
	if !showAll {
		tracked := bcp.Tracking.Channels(i.GuildID)
		if len(tracked) == 0 {
			return "No channels tracked yet!", nil
		}
		lines := make([]string, len(tracked))
		for j, c := range tracked {
			lines[j] = fmt.Sprintf("<#%s>", c)
			if c == main {
				lines[j] = MAIN_MARK + " " + lines[j]
			}
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
		if c.ID == main {
			mark = MAIN_MARK
		} else if bcp.Tracking.IsTracked(i.GuildID, c.ID) {
			mark = "✅"
		}
		lines[j] = fmt.Sprintf("%s <#%s>", mark, c.ID)
	}
	return strings.Join(lines, "\n"), nil
}

func show(s session, i intxn, bcp bcpackage, all bool) (string, bool, error) {
	return "", all, nil
}

// the first channel a guild tracks becomes its main channel
func madeMain(summary string, main string) string {
	if main == "" {
		return summary
	}
	return fmt.Sprintf("%s\n%s <#%s> is now the main channel, where daily results are posted. %s", summary, MAIN_MARK, main, MAIN_HINT)
}

func track(s session, i intxn, bcp bcpackage, all bool) (string, bool, error) {
	if !all {
		added, main, err := bcp.Tracking.Track(i.GuildID, i.ChannelID)
		if err != nil {
			return "", false, err
		}
		if added == 0 {
			return fmt.Sprintf("<#%s> is already tracked.", i.ChannelID), false, nil
		}
		return madeMain(fmt.Sprintf("Now tracking <#%s>.", i.ChannelID), main), false, nil
	}
	channels, err := textChannels(s, i.GuildID)
	if err != nil {
		return "", false, err
	}
	ids := make([]string, len(channels))
	for j, c := range channels {
		ids[j] = c.ID
	}
	added, main, err := bcp.Tracking.Track(i.GuildID, ids...)
	if err != nil {
		return "", false, err
	}
	return madeMain(fmt.Sprintf("Now tracking %d more channel(s).", added), main), false, nil
}

func untrack(s session, i intxn, bcp bcpackage, all bool) (string, bool, error) {
	if all {
		removed, err := bcp.Tracking.UntrackAll(i.GuildID)
		if err != nil {
			return "", false, err
		}
		return fmt.Sprintf("Stopped tracking %d channel(s).", removed), false, nil
	}
	removed, lostMain, err := bcp.Tracking.Untrack(i.GuildID, i.ChannelID)
	if err != nil {
		return "", false, err
	}
	if removed == 0 {
		return fmt.Sprintf("<#%s> wasn't tracked.", i.ChannelID), false, nil
	}
	summary := fmt.Sprintf("Stopped tracking <#%s>.", i.ChannelID)
	if lostMain {
		summary += "\nIt was the main channel, so there's none now and daily results won't be posted. Set one with `/mainchan`."
	}
	return summary, false, nil
}

// always overwrites; the channel is tracked too if it wasn't
func mainchan(s session, i intxn, bcp bcpackage, _ bool) (string, bool, error) {
	old, newlyTracked, err := bcp.Tracking.SetMain(i.GuildID, i.ChannelID)
	if err != nil {
		return "", false, err
	}
	var summary string
	switch old {
	case "":
		summary = fmt.Sprintf("%s <#%s> is now the main channel, where daily results are posted.", MAIN_MARK, i.ChannelID)
	case i.ChannelID:
		summary = fmt.Sprintf("%s <#%s> is already the main channel.", MAIN_MARK, i.ChannelID)
	default:
		summary = fmt.Sprintf("%s Main channel changed: <#%s> → <#%s>", MAIN_MARK, old, i.ChannelID)
	}
	if newlyTracked {
		summary += fmt.Sprintf("\nNow tracking <#%s> too.", i.ChannelID)
	}
	return summary, false, nil
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
				Embeds:          []*discordgo.MessageEmbed{{Title: title, Description: list, Color: theme.ACCENT}},
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
	(*bcp.Handles)["mainchan"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "mainchan",
			Description: "post daily results in this channel (tracks it too)",
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: respond(mainchan, bcp),
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
