package help

import (
	"fmt"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/pages"
	"krill-agg/root/types"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo

type entry struct {
	usage, desc string
}

// a page's worth of .command_help: the commands under one "!Name" header
type section struct {
	name    string
	entries []entry
}

// the pages after Home, in order, with the blurb each opens on.
// .command_help must have exactly these headers
var layout = []struct{ name, blurb string }{
	{"Game Info", "The daily Krillion itself: what's live, how each day went, and how scores here stack up against everyone else's."},
	{"Player Stats", "Individual results, leaderboards and head-to-heads, plus pulling your own scores back out."},
	{"Admin", "Server setup and upkeep: tracked channels, the daily report, leaderboard upkeep and bot ranks. Needs a manager rank."},
	{"Miscellaneous", "Everything else: tracked channels, ranks and checking on the bot."},
}

// reads .command_help: "!Name" headers, each followed by its entries, three
// lines each: the usage ("/cmd [needed] || [optional]"), a one-sentence
// description, then a blank line. a malformed one is an error, so a typo shows
// up in tests rather than as a garbled /help
func parse(raw string) ([]section, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	var sections []section
	for j := 0; j < len(lines); {
		line := strings.TrimSpace(lines[j])
		if line == "" && j == len(lines)-1 {
			break // the file's final newline
		}
		if name, ok := strings.CutPrefix(line, "!"); ok {
			if name = strings.TrimSpace(name); name == "" {
				return nil, fmt.Errorf("line %d: section header has no name", j+1)
			}
			if n := len(sections); n > 0 && len(sections[n-1].entries) == 0 {
				return nil, fmt.Errorf("line %d: section %q is empty", j+1, sections[n-1].name)
			}
			sections = append(sections, section{name: name})
			j++
			continue
		}
		if len(sections) == 0 {
			return nil, fmt.Errorf("line %d: %q comes before any !section", j+1, line)
		}
		if j+1 >= len(lines) {
			return nil, fmt.Errorf("line %d: %q has no description", j+1, line)
		}
		desc := strings.TrimSpace(lines[j+1])
		if !strings.HasPrefix(line, "/") || desc == "" || strings.HasPrefix(desc, "!") {
			return nil, fmt.Errorf("line %d: want a /command then its description, got %q / %q", j+1, line, desc)
		}
		if j+2 < len(lines) && strings.TrimSpace(lines[j+2]) != "" {
			return nil, fmt.Errorf("line %d: entries are separated by a blank line", j+3)
		}
		cur := &sections[len(sections)-1]
		cur.entries = append(cur.entries, entry{line, desc})
		j += 3
	}
	if n := len(sections); n > 0 && len(sections[n-1].entries) == 0 {
		return nil, fmt.Errorf("section %q is empty", sections[n-1].name)
	}
	return sections, nil
}

// lines the parsed sections up with layout: same names, each exactly once
func arrange(sections []section) ([]section, error) {
	byName := map[string]section{}
	for _, sec := range sections {
		if _, dup := byName[sec.name]; dup {
			return nil, fmt.Errorf("section %q listed twice", sec.name)
		}
		byName[sec.name] = sec
	}
	ordered := make([]section, len(layout))
	for j, l := range layout {
		sec, ok := byName[l.name]
		if !ok {
			return nil, fmt.Errorf("missing section %q", l.name)
		}
		ordered[j] = sec
		delete(byName, l.name)
	}
	for name := range byName {
		return nil, fmt.Errorf("unknown section %q", name)
	}
	return ordered, nil
}

// embed descriptions top out at 4096 characters
const MAX_DESCRIPTION = 4096

const usageHint = "[needed] || [optional]"

func homeBody() string {
	var b strings.Builder
	b.WriteString("Welcome! This bot reads the Krillion results you post here and keeps this server's stats, leaderboards and daily cards. Flip through the pages with the arrows below.\n")
	b.WriteString("\n**1. Home** - you are here")
	for j, l := range layout {
		fmt.Fprintf(&b, "\n**%d. %s** - %s", j+2, l.name, l.blurb)
	}
	fmt.Fprintf(&b, "\n\nArguments read as `%s`.", usageHint)
	return b.String()
}

func sectionBody(blurb string, entries []entry) string {
	blocks := make([]string, len(entries)+1)
	blocks[0] = "*" + blurb + "*"
	for j, e := range entries {
		blocks[j+1] = fmt.Sprintf("**`%s`**\n%s", e.usage, e.desc)
	}
	return strings.Join(blocks, "\n\n")
}

// the book's embeds, Home first. they never change, so they're built once
func helpBook(sections []section) []pages.Page {
	bodies := []struct{ title, body string }{{"Home", homeBody()}}
	for j, sec := range sections {
		bodies = append(bodies, struct{ title, body string }{sec.name, sectionBody(layout[j].blurb, sec.entries)})
	}
	book := make([]pages.Page, len(bodies))
	for j, p := range bodies {
		book[j] = func(at pages.Info) (*discordgo.MessageEmbed, error) {
			embed := pages.Embed("Help · "+p.title, "", p.body, at)
			embed.Footer.Text += " · " + usageHint
			return embed, nil
		}
	}
	return book
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	sections, err := parse(bcp.HelpText)
	if err == nil {
		sections, err = arrange(sections)
	}
	if err != nil {
		panic(fmt.Sprintf(".command_help: %v", err)) // caught by the tests before any release
	}
	book := helpBook(sections)
	(*bcp.Handles)["help"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "help",
			Description: "page through every command",
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			return bcp.Pages.Open(s, i, "help", book)
		},
	}
}
