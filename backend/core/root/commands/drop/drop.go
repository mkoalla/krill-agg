// Package drop is /drop: deleting your own results, one day or all of them,
// behind Confirm clicks that each have to land in time (see DROP).
package drop

import (
	"errors"
	"fmt"
	"krill-agg/root/info/run"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo

// how long a step waits for an answer unless a flow says otherwise
const DEFAULT_TIMEOUT = 30 * time.Second

// how a confirmation runs: Steps Confirm clicks, each with its own Timeout.
// the clock starts over after every click
type Flow struct {
	Steps   int
	Timeout time.Duration // zero means DEFAULT_TIMEOUT
}

func (f Flow) timeout() time.Duration {
	if f.Timeout <= 0 {
		return DEFAULT_TIMEOUT
	}
	return f.Timeout
}

// /drop's: two clicks, "sure" then "REALLY sure"
var DROP = Flow{Steps: 2, Timeout: DEFAULT_TIMEOUT}

// button CustomIDs: "drop:<run>:<owner>:<prompt>:<confirm|cancel>"
const PREFIX = "drop:"

const (
	confirm = "confirm"
	cancel  = "cancel"
)

var noPings = &discordgo.MessageAllowedMentions{}

// one /drop waiting on its owner. whoever takes it done first (a click or
// the timer) is the only one that answers
type prompt struct {
	mu    sync.Mutex
	flow  Flow
	id    uint64
	owner string
	guild string
	what  string    // "ALL your scores", "your score for day 124"
	shown string    // the score being dropped, for a single day
	day   int       // 0: every day
	step  int       // 1-based
	until time.Time // when this step runs out
	timer *time.Timer
	done  bool
}

// starts the step's clock over; caller holds p.mu
func (p *prompt) restart(now time.Time) {
	p.until = now.Add(p.flow.timeout())
	if p.timer != nil {
		p.timer.Reset(p.flow.timeout())
	}
}

var (
	nextID  atomic.Uint64
	mu      sync.Mutex
	prompts = map[uint64]*prompt{}
)

func customID(p *prompt, action string) string {
	return fmt.Sprintf("%s%s:%s:%d:%s", PREFIX, run.ID, p.owner, p.id, action)
}

func buttons(p *prompt) *[]discordgo.MessageComponent {
	return &[]discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Confirm", Style: discordgo.DangerButton, CustomID: customID(p, confirm)},
			discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: customID(p, cancel)},
		}},
	}
}

func noButtons() *[]discordgo.MessageComponent {
	return &[]discordgo.MessageComponent{}
}

// the question for the step p is on, with when it runs out
func question(p *prompt, expiry time.Time) string {
	sure := "sure"
	if p.step > 1 {
		sure = "REALLY sure"
	}
	q := fmt.Sprintf("Are you %s you want to drop %s? (%d/%d)", sure, p.what, p.step, p.flow.Steps)
	if p.shown != "" {
		q += "\n- " + p.shown
	}
	return q + fmt.Sprintf("\n-# expires <t:%d:R>", expiry.Unix())
}

// the first answer to /drop: buttons for a drop that has something to drop,
// or just why not
func start(bcp *types.BotCommandPackage, s *discordgo.Session, i *discordgo.InteractionCreate, day int) error {
	owner, guild := i.Member.User.ID, i.GuildID
	p := &prompt{flow: DROP, id: nextID.Add(1), owner: owner, guild: guild, day: day, step: 1}
	if day == 0 {
		n, err := bcp.Scores.Count(guild, owner)
		if err != nil {
			return err
		}
		if n == 0 {
			return respond(s, i, "You don't have any scores to drop.", nil)
		}
		p.what = "ALL your scores"
		p.shown = fmt.Sprintf("%d score%s", n, plural(n))
	} else {
		score, ok, err := bcp.Scores.Get(guild, day, owner)
		if err != nil {
			return err
		}
		if !ok {
			return respond(s, i, "You don't have a score for this day.", nil)
		}
		p.what = fmt.Sprintf("your score for day %d", day)
		p.shown = score.Format()
	}

	mu.Lock()
	prompts[p.id] = p
	mu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restart(time.Now())
	if err := respond(s, i, question(p, p.until), buttons(p)); err != nil {
		p.finish()
		return err
	}
	// out of time: answered on the original reply, which outlives any click.
	// a click that restarted the clock while this was waiting on the lock wins
	p.timer = time.AfterFunc(p.flow.timeout(), func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.done || time.Now().Before(p.until) {
			return
		}
		p.finish()
		content := "Time's up, nothing was dropped."
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, Components: noButtons(), AllowedMentions: noPings}) //nolint // nobody left to tell
	})
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components *[]discordgo.MessageComponent) error {
	data := &discordgo.InteractionResponseData{Content: content, AllowedMentions: noPings}
	if components != nil {
		data.Components = *components
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	})
}

// takes p out of play; caller holds p.mu
func (p *prompt) finish() {
	p.done = true
	if p.timer != nil {
		p.timer.Stop()
	}
	mu.Lock()
	delete(prompts, p.id)
	mu.Unlock()
}

// a click's run, owner, prompt and action, if it's one of ours
func parseID(cid string) (runID, owner string, id uint64, action string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(cid, PREFIX), ":")
	if len(parts) != 4 {
		return "", "", 0, "", false
	}
	id, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return "", "", 0, "", false
	}
	return parts[0], parts[1], id, parts[3], true
}

// the prefix handler. clicks are already acknowledged (deferred update);
// only the owner's do anything
func click(bcp *types.BotCommandPackage) func(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error) {
		if i.Type != discordgo.InteractionMessageComponent || i.Member == nil {
			return true, nil
		}
		runID, owner, id, action, ok := parseID(i.MessageComponentData().CustomID)
		if !ok || i.Member.User.ID != owner {
			return true, nil
		}
		edit := func(content string, components *[]discordgo.MessageComponent) error {
			_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, Components: components, AllowedMentions: noPings})
			return err
		}
		mu.Lock()
		p, live := prompts[id]
		mu.Unlock()
		if runID != run.ID || !live {
			// from before a restart, or already answered: close it out
			return true, edit("-# This prompt has expired.", noButtons())
		}

		p.mu.Lock()
		defer p.mu.Unlock()
		// past the step's time: the timer answers, even if it hasn't yet
		if p.done || !time.Now().Before(p.until) {
			return true, nil
		}
		switch {
		case action == cancel:
			p.finish()
			return true, edit("Cancelled, nothing was dropped.", noButtons())
		case p.step < p.flow.Steps:
			// the clock starts over for the next step
			p.step++
			p.restart(time.Now())
			return true, edit(question(p, p.until), buttons(p))
		}
		p.finish()
		result, err := p.drop(bcp)
		if err != nil {
			return true, errors.Join(fmt.Errorf("dropping %s: %v", p.what, err), edit("Couldn't drop that, try again later.", noButtons()))
		}
		// the boards would keep showing it until the hour turns otherwise
		return true, errors.Join(edit(result, noButtons()), bcp.Leaderboard.Refetch(p.guild))
	}
}

// does the drop; says what was dropped
func (p *prompt) drop(bcp *types.BotCommandPackage) (string, error) {
	if p.day == 0 {
		n, err := bcp.Scores.DropAll(p.guild, p.owner)
		return fmt.Sprintf("Dropped all %d of your score%s.", n, plural(n)), err
	}
	ok, err := bcp.Scores.Drop(p.guild, p.owner, p.day)
	if err == nil && !ok {
		return "You don't have a score for this day anymore.", nil
	}
	return fmt.Sprintf("Dropped your score for day %d.\n- %s", p.day, p.shown), err
}

func Register(bcp *types.BotCommandPackage) {
	bcp.Components.AddPrefixHandler(PREFIX, click(bcp))
	(*bcp.Handles)["drop"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "drop",
			Description: "delete your own results in this server",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "day",
				Description: "krillion # to drop (default: all of them)",
				MinValue:    &minDay,
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			day := 0
			for _, opt := range i.ApplicationCommandData().Options {
				if opt.Name == "day" {
					day = int(opt.IntValue())
				}
			}
			return start(bcp, s, i, day)
		},
	}
}

// krillion #1 is the first puzzle
var minDay = 1.0
