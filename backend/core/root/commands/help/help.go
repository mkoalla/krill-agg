package help

import (
	"fmt"
	"krill-agg/root/info/theme"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo

type entry struct {
	usage, desc string
}

// reads .command_help's entries, three lines each: the usage
// ("/cmd [needed] || [optional]"), a one-sentence description, then a blank
// line. a malformed one is an error, so a typo shows up in tests rather than
// as a garbled /help
func parse(raw string) ([]entry, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	var entries []entry
	for j := 0; j < len(lines); j += 3 {
		if strings.TrimSpace(lines[j]) == "" && j == len(lines)-1 {
			break // the file's final newline
		}
		if j+1 >= len(lines) {
			return nil, fmt.Errorf("line %d: %q has no description", j+1, lines[j])
		}
		usage, desc := strings.TrimSpace(lines[j]), strings.TrimSpace(lines[j+1])
		if !strings.HasPrefix(usage, "/") || desc == "" {
			return nil, fmt.Errorf("line %d: want a /command then its description, got %q / %q", j+1, usage, desc)
		}
		if j+2 < len(lines) && strings.TrimSpace(lines[j+2]) != "" {
			return nil, fmt.Errorf("line %d: entries are separated by a blank line", j+3)
		}
		entries = append(entries, entry{usage, desc})
	}
	return entries, nil
}

// embed descriptions top out at 4096 characters
const MAX_DESCRIPTION = 4096

func helpEmbed(entries []entry) *discordgo.MessageEmbed {
	blocks := make([]string, len(entries))
	for j, e := range entries {
		blocks[j] = fmt.Sprintf("**`%s`**\n%s", e.usage, e.desc)
	}
	return &discordgo.MessageEmbed{
		Title:       "Commands",
		Description: strings.Join(blocks, "\n\n"),
		Footer:      &discordgo.MessageEmbedFooter{Text: "[needed] || [optional]"},
		Color:       theme.ACCENT,
	}
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	entries, err := parse(bcp.HelpText)
	if err != nil {
		panic(fmt.Sprintf(".command_help: %v", err)) // caught by the tests before any release
	}
	embed := helpEmbed(entries)
	(*bcp.Handles)["help"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "help",
			Description: "list every command",
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
			})
		},
	}
}
