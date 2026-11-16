package bot

import (
	"errors"
	"fmt"
	"krill-agg/root/managers/audit"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
)

// message content is privileged and must be enabled for the bot in the developer portal
const INTENTS = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

// gateway close code for privileged intents the app isn't approved for
const CLOSE_DISALLOWED_INTENTS = 4014

type StartInstructions = types.StartInstructions
type Session = types.Session

func getCommandValue(opt *discordgo.ApplicationCommandInteractionDataOption) any {
	switch opt.Type {
	case discordgo.ApplicationCommandOptionString:
		return fmt.Sprintf("%q", opt.StringValue())
	case discordgo.ApplicationCommandOptionInteger:
		return opt.IntValue()
	case discordgo.ApplicationCommandOptionBoolean:
		return opt.BoolValue()
	case discordgo.ApplicationCommandOptionNumber:
		return opt.FloatValue()
	default:
		return opt.Value
	}
}
func rebuildCommand(c *discordgo.ApplicationCommandInteractionData) string {
	args := make([]string, len(c.Options)+1)
	args[0] = fmt.Sprintf("/%s", c.Name)
	for i, option := range c.Options {
		args[i+1] = fmt.Sprintf("%s:%v", option.Name, getCommandValue(option))
	}
	return strings.Join(args, " ")
}

func ack(s *discordgo.Session, i *discordgo.InteractionCreate, logger *audit.Log, value discordgo.InteractionResponseType, flags discordgo.MessageFlags) {
	resp := &discordgo.InteractionResponse{
		Type: value,
	}
	// flags belong to the deferred message; a modal response can't carry them
	if flags != 0 {
		resp.Data = &discordgo.InteractionResponseData{Flags: flags}
	}
	err := s.InteractionRespond(i.Interaction, resp)
	if err != nil {
		logger.Warn(fmt.Sprintf("failed to defer: %s", err), audit.LogGroup.BOT, audit.LogGroup.INTERACT)
		return
	}
}

type dgSession = discordgo.Session
type dgInteract = discordgo.InteractionCreate
type runner = func() error

type autoAck struct {
	Acknowledge discordgo.InteractionResponseType
	// flags for the auto-acknowledged (deferred) message, e.g. ephemeral
	Flags   discordgo.MessageFlags
	Checker func(instr *StartInstructions, s *dgSession, i *dgInteract) (msg string, runner runner)
}

func noOp() error { return nil }
func wrap(err error) runner {
	return func() error { return err }
}

var dgInteractHandlers = map[discordgo.InteractionType]*autoAck{
	discordgo.InteractionApplicationCommand: {
		Acknowledge: 0,
		Checker: func(instr *StartInstructions, s *dgSession, i *dgInteract) (msg string, runner runner) {
			runner = noOp

			bcp := instr.Commands
			commandData := i.ApplicationCommandData()
			handle, ok := (*bcp.Handles)[commandData.Name]
			if !ok {
				runner = wrap(fmt.Errorf("unknown command: /%s", commandData.Name))
				return
			}
			id := i.Member.User.ID

			cmd := rebuildCommand(&commandData)
			var callback types.EventCallback
			var statusChar string
			if !bcp.Auth.CanUse(id, handle.Metadata.MinTrustLevel) {
				statusChar = "❌"
				callback = authorize.PermissionsErrorIntercept
			} else {
				statusChar = "✅"
				callback = handle.Callback
			}

			if err := callback(s, i); err != nil {
				runner = wrap(fmt.Errorf("while handling %s: %v", rebuildCommand(&commandData), err))
				return
			}

			msg = fmt.Sprintf("%v %02d %s| %s", id, bcp.Auth.GetRank(id), statusChar, cmd)
			return
		},
	},
	discordgo.InteractionMessageComponent: {
		Acknowledge: discordgo.InteractionResponseDeferredMessageUpdate,
		Checker: func(instr *StartInstructions, s *dgSession, i *dgInteract) (msg string, runner runner) {
			runner = noOp
			bcp := instr.Commands
			id := i.Member.User.ID

			// leftovers (e.g. from a previous run) are claimed before normal routing
			if handled, err := bcp.Components.TryOrphan(s, i); handled || err != nil {
				msg = fmt.Sprintf("%v %02d  | -> orphan %s", id, bcp.Auth.GetRank(id), i.MessageComponentData().CustomID)
				if err != nil {
					runner = wrap(fmt.Errorf("while handling orphan Component %s: %v", i.MessageComponentData().CustomID, err))
				}
				return
			}

			group, ok := bcp.Components.GroupOf(id)
			if !ok {
				runner = wrap(errors.New("no group assigned to this interaction"))
				return
			}

			componentData := i.MessageComponentData()
			var statusChar string
			metadata, err := bcp.Components.GetMetadata(group)
			if err != nil {
				return
			}

			canUse := bcp.Auth.CanUse(id, (*bcp.Handles)[metadata.FromHandle].Metadata.MinTrustLevel)
			if !canUse {
				statusChar = "❌"
			} else {
				statusChar = "✅"
			}
			msg = fmt.Sprintf("%v %02d %s| -> %s", id, bcp.Auth.GetRank(id), statusChar, componentData.CustomID)
			if !canUse {
				return
			}
			busy, err := bcp.Components.TryRun(i)
			if err != nil {
				runner = wrap(fmt.Errorf("while handling Component %s: %v", componentData.CustomID, err))
			} else if busy {
				runner = wrap(errors.New("component command ignored (busy)"))
			}
			return
		},
	},
	discordgo.InteractionModalSubmit: {
		Acknowledge: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Flags:       discordgo.MessageFlagsEphemeral,
		Checker: func(instr *StartInstructions, s *dgSession, i *dgInteract) (msg string, runner runner) {
			runner = noOp
			id := i.Member.User.ID
			bcp := instr.Commands
			group, ok := bcp.Components.GroupOf(id)
			if !ok {
				runner = wrap(errors.New("no group assigned to this interaction"))
				return
			}

			componentData := i.ModalSubmitData()
			var statusChar string
			metadata, err := bcp.Components.GetMetadata(group)
			if err != nil {
				return
			}
			canUse := bcp.Auth.CanUse(id, (*bcp.Handles)[metadata.FromHandle].Metadata.MinTrustLevel)
			if !canUse {
				statusChar = "❌"
			} else {
				statusChar = "✅"
			}
			msg = fmt.Sprintf("%v %02d %s| Modal-> %s", id, bcp.Auth.GetRank(id), statusChar, componentData.CustomID)
			if !canUse {
				return
			}
			busy, err := bcp.Components.TryRun(i)
			if err != nil {
				runner = wrap(fmt.Errorf("while handling Modal %s: %v", componentData.CustomID, err))
			} else if busy {
				runner = wrap(fmt.Errorf("component command ignored (busy)"))
			}
			return
		},
	},
}

// 2. Initialize a new Discord
func Start(instr StartInstructions) (session *Session, err error) {

	token, handles, logger := instr.Token, instr.Commands.Handles, instr.Logger
	dgSession, err := discordgo.New("Bot " + token)
	if err != nil {
		return
	}

	logger.Add("Begin bot init", audit.LogGroup.BOT)
	// set perms "intents" & register gateway handler boilerplate
	dgSession.Identify.Intents = INTENTS
	dgSession.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		handle, ok := dgInteractHandlers[i.Type]
		if !ok {
			return
		}
		if handle.Acknowledge != 0 {
			// synchronous: handlers edit the response we just deferred
			ack(s, i, logger, handle.Acknowledge, handle.Flags)
		}

		//
		msg, runner := handle.Checker(&instr, s, i)
		logger.Add(msg, audit.LogGroup.BOT, audit.LogGroup.INTERACT)
		if err := runner(); err != nil {
			logger.Warn(err, audit.LogGroup.BOT, audit.LogGroup.INTERACT)
		}
	})

	// plain messages don't go through the interaction router; they're their own gateway event
	dgSession.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author == nil || m.Author.Bot {
			return
		}
		for _, handler := range *instr.Commands.MessageHandlers {
			msg, err := handler(s, m)
			if msg != "" {
				id := m.Author.ID
				logger.Add(fmt.Sprintf("%v %02d  | %s", id, instr.Commands.Auth.GetRank(id), msg), audit.LogGroup.BOT, audit.LogGroup.MESSAGE)
			}
			if err != nil {
				logger.Warn(err, audit.LogGroup.BOT, audit.LogGroup.MESSAGE)
			}
		}
	})

	// take a wild guess
	err = dgSession.Open()
	if err != nil {
		// discord rejects privileged intents that aren't toggled on in the developer portal
		var closeErr *websocket.CloseError
		if errors.As(err, &closeErr) && closeErr.Code == CLOSE_DISALLOWED_INTENTS {
			logger.Panic(fmt.Sprintf("Missing privileged intents (enable Message Content Intent under Bot in the Discord Developer Portal): %v", err), audit.LogGroup.BOT)
		}
		return
	}

	// register slash commands
	var registeredCommands []*discordgo.ApplicationCommand
	{
		payload := make([]*discordgo.ApplicationCommand, len(*handles))
		i := 0
		for _, cmd := range *handles {
			payload[i] = cmd.DGInfo
			i++
		}
		registeredCommands, err = dgSession.ApplicationCommandBulkOverwrite(dgSession.State.User.ID, "", payload)
		if err != nil {
			logger.Panic(fmt.Sprintf("Failed to create commands: %v", err), audit.LogGroup.BOT)
		}
	}

	session = &Session{
		DGSession: dgSession,
		Registers: registeredCommands,
		Logger:    logger,
		Package:   instr.Commands,
	}
	logger.Add("Init success; bot online", audit.LogGroup.BOT)
	return
}

func EndSession(session *Session) {
	session.Logger.Add("Ending session...", audit.LogGroup.BOT)
	defer session.Package.Auth.Close()     //nolint
	defer session.Package.Scores.Close()   //nolint
	defer session.Package.Tracking.Close() //nolint

	defer func() {
		if err := session.DGSession.Close(); err != nil {
			session.Logger.Warn(fmt.Sprintf("Error while closing DGSession: %v", err))
		}
		session.Logger.Add("Session ended", audit.LogGroup.BOT)
	}()
}
