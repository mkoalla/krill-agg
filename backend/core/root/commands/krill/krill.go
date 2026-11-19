package krill

import (
	"errors"
	"fmt"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/krills"
	"krill-agg/root/managers/pages"
	"krill-agg/root/types"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type Krill = krills.Krill

// the krillion tier's emoji (krillion.EmojiValues: 100 points)
const STAR = "🌟"

// answers are listed with a plain dot, not a markdown "- " (which would
// start a list, and lists break multi-line spoilers)
const DOT = "•"

const (
	NOT_COLLECTED = "Not collected yet! Krillion only reveals a day's answers on the day itself."
	NO_KRILLIONS  = "No krillion " + STAR + " answers today!"
	NO_QUIPS      = "No quips today!"
)

// embed descriptions top out at 4096 characters
const MAX_DESCRIPTION = 4096

// cut by character, never through the middle of one (emoji are several bytes)
func clip(s string) string {
	r := []rune(s)
	if len(r) <= MAX_DESCRIPTION {
		return s
	}
	return string(r[:MAX_DESCRIPTION-1]) + "…"
}

func spoil(s string) string { return "||" + s + "||" }

// per question, its number then one spoiler around the question and its
// "• answer" lines: "1\\. ||question\n• answer||" (shows as "1."). a question with no krillion
// answer gets "• N/A" outside the spoiler. questions[j] may be missing, and
// then the block is just the answers
func answersBody(questions []string, day [][]Krill) string {
	any := false
	blocks := make([]string, len(day))
	for j, ks := range day {
		var lines []string
		if j < len(questions) && questions[j] != "" {
			lines = append(lines, questions[j])
		}
		for _, k := range ks {
			lines = append(lines, DOT+" "+k.Answer)
			any = true
		}
		// the dot is escaped: as a real markdown list item, a spoiler can't
		// span lines and each block's || pairs with the next block's
		block := fmt.Sprintf("%d\\. ", j+1)
		if len(lines) > 0 {
			block += spoil(strings.Join(lines, "\n"))
		}
		if len(ks) == 0 {
			if len(lines) > 0 {
				block += "\n"
			}
			block += DOT + " N/A"
		}
		blocks[j] = block
	}
	if !any {
		return NO_KRILLIONS
	}
	return strings.Join(blocks, "\n\n") // a blank line between questions
}

// "answer" then "• quip" for each krillion answer that has a quip, all in
// one spoiler
func quipsBody(day [][]Krill) string {
	var blocks []string
	for _, ks := range day {
		for _, k := range ks {
			if k.Quip != "" {
				blocks = append(blocks, k.Answer+"\n"+DOT+" "+k.Quip)
			}
		}
	}
	if len(blocks) == 0 {
		return NO_QUIPS
	}
	return spoil(strings.Join(blocks, "\n\n"))
}

// two pages: the answers, then the quips
func book(dayNum int, questions []string, day [][]Krill) []pages.Page {
	page := func(title, body string) pages.Page {
		return func(at pages.Info) (*discordgo.MessageEmbed, error) {
			return pages.Embed(title, "", body, at), nil
		}
	}
	return []pages.Page{
		page(fmt.Sprintf("%s 100 %s Krillion #%d %s", STAR, STAR, dayNum, krillion.HEADER_EMOJI), clip(answersBody(questions, day))),
		page(fmt.Sprintf("Quips · Krillion #%d (click to show all)", dayNum), clip(quipsBody(day))),
	}
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	(*bcp.Handles)["krill"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "krill",
			Description: "a day's krillion (100 point) answers, spoilered",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "day",
				Description: krillion.DAY_OPTION + " (default: today)",
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			// today's may need a call to krillion first
			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			}); err != nil {
				return err
			}
			day := krillion.Today()
			for _, opt := range i.ApplicationCommandData().Options {
				if opt.Name == "day" {
					day = krillion.Resolve(int(opt.IntValue()))
				}
			}
			edit := func(e *discordgo.WebhookEdit) error {
				_, err := s.InteractionResponseEdit(i.Interaction, e)
				return err
			}

			answers, err := bcp.Krills.Fetch(day)
			if errors.Is(err, krills.ErrNotToday) {
				content := NOT_COLLECTED
				return edit(&discordgo.WebhookEdit{Content: &content})
			}
			if err != nil {
				content := "❌ couldn't get today's answers: " + err.Error()
				return errors.Join(err, edit(&discordgo.WebhookEdit{Content: &content}))
			}
			questions, ok := bcp.Questions.Get(day)
			if !ok && day == krillion.Today() {
				// not saved yet: today's are still on offer, so fetch and keep them
				questions, _ = bcp.Questions.Fetch(day) //nolint // optional: a block just goes without its question
			}
			return bcp.Pages.OpenDeferred(s, i, "krill", book(day, questions, answers))
		},
	}
}
