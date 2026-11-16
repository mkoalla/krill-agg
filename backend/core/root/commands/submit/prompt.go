package submit

import (
	"fmt"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

const (
	updateGroup = "krill-update"
	customYes   = updateGroup + "-yes"
	customNo    = updateGroup + "-no"
	customClose = updateGroup + "-close"
)

// FromHandle only supplies the auth rank check; "stats" is DEFAULT, same as submitting
func registerUpdateGroup(bcp *types.BotCommandPackage) {
	bcp.Components.AddGroup(updateGroup, &components.GroupMetadata{
		FromHandle:         "stats",
		NewInvalidatesOld:  true,
		InvalidationCloses: true,
	})
}

// one pending update prompt
type promptState struct {
	userID     string
	guildID    string
	channelID  string
	promptID   string // the prompt message (buttons live here)
	originalID string // the krillion message that triggered the conflict
	newScore   krillion.PlayerScore
	done       atomic.Bool // guards against the close callback running twice
}

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

// sends the update/cancel prompt and wires up its buttons
func startUpdatePrompt(bcp *types.BotCommandPackage, s *discordgo.Session, m *discordgo.MessageCreate, result scores.Result) error {
	content := fmt.Sprintf("Update your score for day %d? Current: %s; New: %s", result.Score.Day, result.Existing.Format(), result.Score.Format())
	msg, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:         content,
		Reference:       m.Reference(),
		AllowedMentions: noMentions,
		Components:      updateButtons(),
	})
	if err != nil {
		return err
	}

	state := &promptState{
		userID:     m.Author.ID,
		guildID:    m.GuildID,
		channelID:  m.ChannelID,
		promptID:   msg.ID,
		originalID: m.ID,
		newScore:   result.Score,
	}

	return bcp.Components.Register(updateGroup, components.NewComponentCallbacks(m.Author.ID, map[string]components.Callback{
		customYes:   yesCallback(bcp, s, state),
		customNo:    noCallback(s, state),
		customClose: closeCallback(s, state),
	}))
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

func yesCallback(bcp *types.BotCommandPackage, s *discordgo.Session, state *promptState) components.Callback {
	return func(i *discordgo.InteractionCreate) (bool, error) {
		if stale(i, state.promptID) {
			return false, nil
		}

		bcp.Scores.Set(state.guildID, state.userID, state.newScore)

		content := fmt.Sprintf("Updated day %d: %s", state.newScore.Day, state.newScore.Format())
		if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content:    &content,
			Components: clearComponents(),
		}); err != nil {
			return true, err
		}
		if err := s.MessageReactionRemove(state.channelID, state.originalID, emojiConflict, "@me"); err != nil {
			return true, err
		}
		if err := s.MessageReactionAdd(state.channelID, state.originalID, emojiStored); err != nil {
			return true, err
		}
		return true, nil
	}
}

func noCallback(s *discordgo.Session, state *promptState) components.Callback {
	return func(i *discordgo.InteractionCreate) (bool, error) {
		if stale(i, state.promptID) {
			return false, nil
		}

		content := fmt.Sprintf("Kept existing score for day %d.", state.newScore.Day)
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content:    &content,
			Components: clearComponents(),
		})
		return true, err
	}
}

// close runs twice in the normal flow: once immediately after yes/no returns
// done=true (with that same click's interaction, nothing left to do), and
// once if a newer prompt from the same user invalidates this one before it's
// resolved (called with a nil interaction instead). the atomic flag makes it
// idempotent regardless of which path gets there first.
func closeCallback(s *discordgo.Session, state *promptState) components.Callback {
	return func(i *discordgo.InteractionCreate) (bool, error) {
		if stale(i, state.promptID) {
			return false, nil
		}
		if !state.done.CompareAndSwap(false, true) {
			return true, nil
		}
		if i != nil {
			return true, nil
		}

		content := "-# This prompt has expired."
		_, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:    state.channelID,
			ID:         state.promptID,
			Content:    &content,
			Components: clearComponents(),
		})
		return true, err
	}
}
