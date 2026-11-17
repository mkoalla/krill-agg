// Package confirm asks before something can't be undone: a reply with
// Confirm and Cancel buttons that only the person who asked can click, taking
// Flow.Steps Confirm clicks, each within Flow.Timeout (the clock starts over
// after every click). Replies are embeds, so mentions in them never ping.
package confirm

import (
	"errors"
	"fmt"
	"krill-agg/root/info/run"
	"krill-agg/root/info/theme"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
)

// how long a step waits for an answer unless a flow says otherwise
const DEFAULT_TIMEOUT = 30 * time.Second

// how a confirmation runs: Steps Confirm clicks, each with its own Timeout
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

func (f Flow) steps() int { return max(f.Steps, 1) }

// one question, and what happens when it's answered
type Prompt struct {
	Flow Flow
	// the question at step (1-based); the step count and expiry are added
	Question func(step int) string
	// runs once the last Confirm lands; text replaces the question. a
	// non-nil err is returned for logging, after text is shown
	Confirm func() (text string, err error)
	// shown on Cancel and on running out of time; defaults if empty
	Cancelled string
	TimedOut  string
}

// button CustomIDs: "confirm:<run>:<owner>:<prompt>:<yes|no>"
const PREFIX = "confirm:"

const (
	yes = "yes"
	no  = "no"
)

var noPings = &discordgo.MessageAllowedMentions{}

// one prompt waiting on its owner. whoever takes it done first (a click or
// the timer) is the only one that answers
type pending struct {
	mu    sync.Mutex
	p     Prompt
	id    uint64
	owner string
	step  int       // 1-based
	until time.Time // when this step runs out
	timer *time.Timer
	done  bool
}

var (
	nextID  atomic.Uint64
	mu      sync.Mutex
	prompts = map[uint64]*pending{}
)

// starts the step's clock over; caller holds q.mu
func (q *pending) restart(now time.Time) {
	q.until = now.Add(q.p.Flow.timeout())
	if q.timer != nil {
		q.timer.Reset(q.p.Flow.timeout())
	}
}

// takes q out of play; caller holds q.mu
func (q *pending) finish() {
	q.done = true
	if q.timer != nil {
		q.timer.Stop()
	}
	mu.Lock()
	delete(prompts, q.id)
	mu.Unlock()
}

// the question for the step q is on, with its count and when it runs out
func (q *pending) question() string {
	return fmt.Sprintf("%s (%d/%d)\n-# expires <t:%d:R>", q.p.Question(q.step), q.step, q.p.Flow.steps(), q.until.Unix())
}

func customID(q *pending, action string) string {
	return fmt.Sprintf("%s%s:%s:%d:%s", PREFIX, run.ID, q.owner, q.id, action)
}

func buttons(q *pending) *[]discordgo.MessageComponent {
	return &[]discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Confirm", Style: discordgo.DangerButton, CustomID: customID(q, yes)},
			discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: customID(q, no)},
		}},
	}
}

func noButtons() *[]discordgo.MessageComponent {
	return &[]discordgo.MessageComponent{}
}

// the reply's embed
func Embeds(text string) []*discordgo.MessageEmbed {
	return []*discordgo.MessageEmbed{{Description: text, Color: theme.ACCENT}}
}

// answers the interaction with text alone, no buttons (e.g. nothing to confirm)
func Reply(s *discordgo.Session, i *discordgo.InteractionCreate, text string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: Embeds(text), AllowedMentions: noPings},
	})
}

// answers the interaction with p's first question. only whoever ran it can click
func Open(s *discordgo.Session, i *discordgo.InteractionCreate, p Prompt) error {
	if p.Cancelled == "" {
		p.Cancelled = "Cancelled."
	}
	if p.TimedOut == "" {
		p.TimedOut = "Time's up."
	}
	q := &pending{p: p, id: nextID.Add(1), owner: i.Member.User.ID, step: 1}
	mu.Lock()
	prompts[q.id] = q
	mu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	q.restart(time.Now())
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: Embeds(q.question()), Components: *buttons(q), AllowedMentions: noPings},
	}); err != nil {
		q.finish()
		return err
	}
	// out of time: answered on the original reply, which outlives any click.
	// a click that restarted the clock while this was waiting on the lock wins
	q.timer = time.AfterFunc(p.Flow.timeout(), func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.done || time.Now().Before(q.until) {
			return
		}
		q.finish()
		embeds := Embeds(q.p.TimedOut)
		if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds, Components: noButtons(), AllowedMentions: noPings}); err != nil {
			log.Printf("confirm: timing out prompt %d: %v", q.id, err)
		}
	})
	return nil
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
func click(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error) {
	if i.Type != discordgo.InteractionMessageComponent || i.Member == nil {
		return true, nil
	}
	runID, owner, id, action, ok := parseID(i.MessageComponentData().CustomID)
	if !ok || i.Member.User.ID != owner {
		return true, nil
	}
	edit := func(text string, components *[]discordgo.MessageComponent) error {
		embeds := Embeds(text)
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds, Components: components, AllowedMentions: noPings})
		return err
	}
	mu.Lock()
	q, live := prompts[id]
	mu.Unlock()
	if runID != run.ID || !live {
		// from before a restart, or already answered: close it out
		return true, edit("-# This prompt has expired.", noButtons())
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	// past the step's time: the timer answers, even if it hasn't yet
	if q.done || !time.Now().Before(q.until) {
		return true, nil
	}
	switch {
	case action == no:
		q.finish()
		return true, edit(q.p.Cancelled, noButtons())
	case q.step < q.p.Flow.steps():
		q.step++
		q.restart(time.Now())
		return true, edit(q.question(), buttons(q))
	}
	q.finish()
	text, err := q.p.Confirm()
	return true, errors.Join(err, edit(text, noButtons()))
}

// routes clicks to the prompts; call once at startup
func Register(addPrefix func(prefix string, f func(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error))) {
	addPrefix(PREFIX, click)
}
