package submit

import (
	"errors"
	"fmt"
	"krill-agg/root/info/run"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	updateGroup = "krill-update"
	customYes   = updateGroup + "-yes"
	customNo    = updateGroup + "-no"
	customClose = updateGroup + "-close" // the manager requires exactly this, so no suffix
)

// button IDs carry the run and the prompt's owner: "krill-update-yes:<run>:<uid>".
// the run lets a click on a pre-restart prompt be recognized; discord caps these at 100 chars
func customID(base string, userID string) string {
	return fmt.Sprintf("%s:%s:%s", base, run.ID, userID)
}

// how long a user has to answer before the worker ignores the new score
const PROMPT_TIMEOUT = 30 * time.Second

// FromHandle only supplies the auth rank check; "stats" is DEFAULT, same as submitting.
// no invalidation rules: they sweep the whole group (every user), and the
// manager already replaces a user's entry on Register. epochs handle staleness.
func registerUpdateGroup(bcp *types.BotCommandPackage) {
	bcp.Components.AddGroup(updateGroup, &components.GroupMetadata{
		FromHandle: "stats",
	})
	bcp.Components.AddPrefixHandler(updateGroup+"-", previousRun)
}

// a prompt from before a restart: its state (and score) is gone, so the owner's
// click closes it out; anyone else's click is swallowed. this run's prompts fall through
func previousRun(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error) {
	parts := strings.Split(i.MessageComponentData().CustomID, ":")
	if len(parts) == 3 && parts[1] == run.ID {
		return false, nil
	}
	if len(parts) == 3 && parts[2] != i.Member.User.ID {
		return true, nil
	}

	content := "-# This prompt expired when the bot restarted."
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:    &content,
		Components: clearComponents(),
	}); err != nil {
		return true, err
	}
	// the prompt is a reply to the original submission
	if ref := i.Message.MessageReference; ref != nil {
		if err := s.MessageReactionRemove(ref.ChannelID, ref.MessageID, emojiConflict, "@me"); err != nil {
			return true, err
		}
		return true, s.MessageReactionAdd(ref.ChannelID, ref.MessageID, emojiCanceled)
	}
	return true, nil
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

func updateButtons(userID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Update", Style: discordgo.PrimaryButton, CustomID: customID(customYes, userID)},
				discordgo.Button{Label: "Cancel", Style: discordgo.SecondaryButton, CustomID: customID(customNo, userID)},
			},
		},
	}
}

// "Update your score for day 124?\nCurrent:\n- 300 …\nNew:\n- 415 …"
func updateQuestion(result scores.Result, expiry time.Time) string {
	return fmt.Sprintf("Update your score for day %d?\nCurrent:\n- %s\nNew:\n- %s\n-# expires <t:%d:R>",
		result.Score.Day, result.Existing.Format(), result.Score.Format(), expiry.Unix())
}

func startUpdatePrompt(bcp *types.BotCommandPackage, s *discordgo.Session, m *discordgo.MessageCreate, result scores.Result) error {
	expiry := time.Now().Add(PROMPT_TIMEOUT)
	content := updateQuestion(result, expiry)
	msg, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:         content,
		Reference:       m.Reference(),
		AllowedMentions: noMentions,
		Components:      updateButtons(m.Author.ID),
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
		customID(customYes, m.Author.ID): yesCallback(bcp, state),
		customID(customNo, m.Author.ID):  noCallback(state),
		customClose:                      closeCallback(state),
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

		if err := bcp.Scores.Set(state.guildID, state.userID, *state.score); err != nil {
			content := "Couldn't save your score, try again later."
			_, errEdit := state.session.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content:    &content,
				Components: clearComponents(),
			})
			return true, errors.Join(fmt.Errorf("saving krillion #%d: %v", state.score.Day, err), errEdit)
		}

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
