// Package drop is /drop and /mg-drop: deleting your own results, or (for
// managers) someone else's, one day or all of them, behind Confirm clicks
// that each have to land in time (see DROP).
package drop

import (
	"errors"
	"fmt"
	"krill-agg/root/info/run"
	"krill-agg/root/info/theme"
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

// whose scores a drop is about, worded for the reply: "you" for your own,
// a mention otherwise. replies are embeds, so mentions never ping
type whose struct {
	user string
	self bool
}

// "your" / "<@id>'s"
func (w whose) their() string {
	if w.self {
		return "your"
	}
	return fmt.Sprintf("<@%s>'s", w.user)
}

// "You don't" / "<@id> doesn't"
func (w whose) doesnt() string {
	if w.self {
		return "You don't"
	}
	return fmt.Sprintf("<@%s> doesn't", w.user)
}

// one drop waiting on its owner (who ran it). whoever takes it done first (a
// click or the timer) is the only one that answers
type prompt struct {
	mu     sync.Mutex
	flow   Flow
	id     uint64
	owner  string // who ran it, and the only one whose clicks count
	target whose  // whose scores go
	guild  string
	what   string    // "ALL your scores", "<@id>'s score for day 124"
	shown  string    // the score being dropped, or how many
	day    int       // 0: every day
	step   int       // 1-based
	until  time.Time // when this step runs out
	timer  *time.Timer
	done   bool
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

func embeds(text string) *[]*discordgo.MessageEmbed {
	return &[]*discordgo.MessageEmbed{{Description: text, Color: theme.ACCENT}}
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

// the first answer to a drop: buttons when there's something to drop, or
// just why not. day 0 is every day
func start(bcp *types.BotCommandPackage, s *discordgo.Session, i *discordgo.InteractionCreate, target string, day int) error {
	owner, guild := i.Member.User.ID, i.GuildID
	w := whose{user: target, self: target == owner}
	p := &prompt{flow: DROP, id: nextID.Add(1), owner: owner, target: w, guild: guild, day: day, step: 1}
	if day == 0 {
		n, err := bcp.Scores.Count(guild, target)
		if err != nil {
			return err
		}
		if n == 0 {
			return respond(s, i, w.doesnt()+" have any scores to drop.", nil)
		}
		p.what = "ALL " + w.their() + " scores"
		p.shown = fmt.Sprintf("%d score%s", n, plural(n))
	} else {
		score, ok, err := bcp.Scores.Get(guild, day, target)
		if err != nil {
			return err
		}
		if !ok {
			return respond(s, i, w.doesnt()+" have a score for this day.", nil)
		}
		p.what = fmt.Sprintf("%s score for day %d", w.their(), day)
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
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: embeds("Time's up, nothing was dropped."), Components: noButtons(), AllowedMentions: noPings}) //nolint // nobody left to tell
	})
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, text string, components *[]discordgo.MessageComponent) error {
	data := &discordgo.InteractionResponseData{Embeds: *embeds(text), AllowedMentions: noPings}
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
		edit := func(text string, components *[]discordgo.MessageComponent) error {
			_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: embeds(text), Components: components, AllowedMentions: noPings})
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
	w := p.target
	if p.day == 0 {
		n, err := bcp.Scores.DropAll(p.guild, w.user)
		return fmt.Sprintf("Dropped all %d of %s score%s.", n, w.their(), plural(n)), err
	}
	ok, err := bcp.Scores.Drop(p.guild, w.user, p.day)
	if err == nil && !ok {
		return w.doesnt() + " have a score for this day anymore.", nil
	}
	return fmt.Sprintf("Dropped %s score for day %d.\n- %s", w.their(), p.day, p.shown), err
}

// /mg-drop's day for every day
const ALL_DAYS = -1

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
				MinValue:    &firstDay,
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
			return start(bcp, s, i, i.Member.User.ID, day)
		},
	}
	(*bcp.Handles)["mg-drop"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "mg-drop",
			Description: "delete someone's results in this server",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "whose results",
				Required:    true,
			}, {
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "day",
				Description: "krillion # to drop, or -1 for all of them",
				Required:    true,
				MinValue:    &allDays,
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			var user string
			day := 0
			for _, opt := range i.ApplicationCommandData().Options {
				switch opt.Name {
				case "user":
					user, _ = opt.Value.(string) // raw: UserValue needs a session lookup
				case "day":
					day = int(opt.IntValue())
				}
			}
			switch day {
			case 0:
				return respond(s, i, "Day must be a krillion # (1 or more), or -1 for all of them.", nil)
			case ALL_DAYS:
				day = 0
			}
			return start(bcp, s, i, user, day)
		},
	}
}

// krillion #1 is the first puzzle; /mg-drop also takes ALL_DAYS
var (
	firstDay = 1.0
	allDays  = float64(ALL_DAYS)
)
