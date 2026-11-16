package submit

import (
	"fmt"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

const (
	emojiRejected = "❌"
	emojiStored   = "✅"
	emojiConflict = "⚠️"
	emojiCanceled = "✋"
)

// never ping anyone from a bot-generated reply
var noMentions = &discordgo.MessageAllowedMentions{}

// reacts and replies to a message that failed parsing/validation. not logged.
func reject(s *discordgo.Session, m *discordgo.MessageCreate, reason error) error {
	if err := s.MessageReactionAdd(m.ChannelID, m.ID, emojiRejected); err != nil {
		return err
	}
	_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content:         emojiRejected + " " + reason.Error(),
		Reference:       m.Reference(),
		AllowedMentions: noMentions,
	})
	return err
}

// reacts to a newly stored score
func store(s *discordgo.Session, m *discordgo.MessageCreate, result scores.Result) (string, error) {
	if err := s.MessageReactionAdd(m.ChannelID, m.ID, emojiStored); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s krillion #%d: %s", emojiStored, result.Score.Day, result.Score.Format()), nil
}

// reacts and prompts the user to confirm overwriting their existing entry
func conflict(bcp *types.BotCommandPackage, s *discordgo.Session, m *discordgo.MessageCreate, result scores.Result) (string, error) {
	if err := s.MessageReactionAdd(m.ChannelID, m.ID, emojiConflict); err != nil {
		return "", err
	}
	if err := startUpdatePrompt(bcp, s, m, result); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s krillion #%d: update prompt (current %s; new %s)", emojiConflict, result.Score.Day, result.Existing.Format(), result.Score.Format()), nil
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	registerUpdateGroup(bcp)
	startExpiryWorker()

	*bcp.MessageHandlers = append(*bcp.MessageHandlers, func(s *discordgo.Session, m *discordgo.MessageCreate) (logMsg string, err error) {
		// only read submissions in guild channels picked with /track
		if m.GuildID == "" || !bcp.Tracking.IsTracked(m.GuildID, m.ChannelID) {
			return "", nil
		}
		result := bcp.Scores.Ingest(m.GuildID, m.Author.ID, m.Content)
		switch result.Outcome {
		case scores.Ignored:
			return "", nil
		case scores.Rejected:
			return "", reject(s, m, result.Err)
		case scores.Stored:
			return store(s, m, result)
		case scores.Conflict:
			return conflict(bcp, s, m, result)
		default:
			return "", nil
		}
	})
}
