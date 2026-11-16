package submit

import (
	"fmt"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	updateGroup = "krill-update"
	customYes   = updateGroup + "-yes"
	customNo    = updateGroup + "-no"
	customClose = updateGroup + "-close"
)

// how long a user has to answer before the worker ignores the new score
const PROMPT_TIMEOUT = 2 * time.Minute

// FromHandle only supplies the auth rank check; "stats" is DEFAULT, same as submitting.
// no invalidation rules: they sweep the whole group (every user), and the
// manager already replaces a user's entry on Register. epochs handle staleness.
func registerUpdateGroup(bcp *types.BotCommandPackage) {
	bcp.Components.AddGroup(updateGroup, &components.GroupMetadata{
		FromHandle: "stats",
	})
}

// userID -> *atomic.Uint64, bumped for each new prompt. only the prompt
// holding the current epoch can act; older ones are left untouched on discord
var epochs sync.Map

func epochOf(userID string) *atomic.Uint64 {
	e, _ := epochs.LoadOrStore(userID, new(atomic.Uint64))
	return e.(*atomic.Uint64)
}

// one pending update prompt. whoever wins the resolved CAS (a button or the
// expiry worker) is the only one that touches discord
type promptState struct {
	session    *discordgo.Session
	userID     string
	guildID    string
	channelID  string
	promptID   string // the prompt message (buttons live here)
	originalID string // the krillion message that triggered the conflict
	score      *krillion.PlayerScore
	expiry     time.Time
	epoch      uint64
	resolved   atomic.Bool
}

// false once the same user has a newer prompt
func (state *promptState) current() bool { return epochOf(state.userID).Load() == state.epoch }

func (state *promptState) expired(now time.Time) bool { return now.After(state.expiry) }

// claims the prompt; false means someone else already resolved it
func (state *promptState) resolve() bool { return state.resolved.CompareAndSwap(false, true) }

func updateButtons() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Update", Style: discordgo.PrimaryButton, CustomID: customYes},
				discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: customNo},
			},
		},
	}
}

func startUpdatePrompt(bcp *types.BotCommandPackage, s *discordgo.Session, m *discordgo.MessageCreate, result scores.Result) error {
	expiry := time.Now().Add(PROMPT_TIMEOUT)
	content := fmt.Sprintf("Update your score for day %d? Current: %s; New: %s\n-# expires <t:%d:R>",
		result.Score.Day, result.Existing.Format(), result.Score.Format(), expiry.Unix())
	msg, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:         content,
		Reference:       m.Reference(),
		AllowedMentions: noMentions,
		Components:      updateButtons(),
	})
	if err != nil {
		return err
	}

	score := result.Score
	state := &promptState{
		session:    s,
		userID:     m.Author.ID,
		guildID:    m.GuildID,
		channelID:  m.ChannelID,
		promptID:   msg.ID,
		originalID: m.ID,
		score:      &score,
		expiry:     expiry,
		epoch:      epochOf(m.Author.ID).Add(1),
	}

	if err := bcp.Components.Register(updateGroup, components.NewComponentCallbacks(m.Author.ID, map[string]components.Callback{
		customYes:   yesCallback(bcp, state),
		customNo:    noCallback(state),
		customClose: closeCallback(state),
	})); err != nil {
		return err
	}
	expiries <- state
	return nil
}

// true when a click belongs to a different (stale) prompt than the one that
// registered this callback set. the manager keys on the clicker's user ID, so
// a user with their own pending prompt could otherwise drive someone else's.
func stale(i *discordgo.InteractionCreate, promptID string) bool {
	return i != nil && i.Message != nil && i.Message.ID != promptID
}

func clearComponents() *[]discordgo.MessageComponent {
	empty := make([]discordgo.MessageComponent, 0)
	return &empty
}

// swaps the caution reaction on the original message for the outcome
func swapReaction(state *promptState, emoji string) error {
	s := state.session
	if err := s.MessageReactionRemove(state.channelID, state.originalID, emojiConflict, "@me"); err != nil {
		return err
	}
	return s.MessageReactionAdd(state.channelID, state.originalID, emoji)
}

// resolves a prompt without an interaction to answer (expiry)
func ignore(state *promptState, content string) error {
	if _, err := state.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    state.channelID,
		ID:         state.promptID,
		Content:    &content,
		Components: clearComponents(),
	}); err != nil {
		return err
	}
	return swapReaction(state, emojiCanceled)
}

// a click past the expiry is left entirely to the worker, even if it hasn't swept yet
func yesCallback(bcp *types.BotCommandPackage, state *promptState) components.Callback {
	return func(i *discordgo.InteractionCreate) (bool, error) {
		if stale(i, state.promptID) || !state.current() || state.expired(time.Now()) || !state.resolve() {
			return false, nil
		}

		bcp.Scores.Set(state.guildID, state.userID, *state.score)

		content := fmt.Sprintf("Updated day %d: %s", state.score.Day, state.score.Format())
		if _, err := state.session.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content:    &content,
			Components: clearComponents(),
		}); err != nil {
			return true, err
		}
		return true, swapReaction(state, emojiStored)
	}
}

func noCallback(state *promptState) components.Callback {
	return func(i *discordgo.InteractionCreate) (bool, error) {
		if stale(i, state.promptID) || !state.current() || state.expired(time.Now()) || !state.resolve() {
			return false, nil
		}

		content := fmt.Sprintf("Kept existing score for day %d.", state.score.Day)
		if _, err := state.session.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content:    &content,
			Components: clearComponents(),
		}); err != nil {
			return true, err
		}
		return true, swapReaction(state, emojiCanceled)
	}
}

// required by the manager; only reached right after yes/no returns done=true,
// which already resolved everything
func closeCallback(state *promptState) components.Callback {
	return func(i *discordgo.InteractionCreate) (bool, error) {
		return true, nil
	}
}
