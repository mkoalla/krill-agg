package backfill

import (
	"errors"
	"fmt"
	"krill-agg/root/info/unix"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/types"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

type session = *discordgo.Session
type intxn = *discordgo.InteractionCreate
type bcpackage = *types.BotCommandPackage

var metadata = types.CommandMetadata{
	MinTrustLevel: authorize.MANAGE,
}

const (
	modalID      = "backfill-modal"
	cancelPrefix = "backfill-cancel:" // + job id
	fieldDay     = "day"
	fieldMonth   = "month"
	fieldYear    = "year"
)

func textField(id string, label string, value string, maxLen int) discordgo.ActionsRow {
	return discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.TextInput{
			CustomID:  id,
			Label:     label,
			Style:     discordgo.TextInputShort,
			Value:     value,
			Required:  true,
			MinLength: 1,
			MaxLength: maxLen,
		},
	}}
}

// prefilled with today (UTC)
func dateModal() *discordgo.InteractionResponseData {
	y, m, d := time.Now().UTC().Date()
	return &discordgo.InteractionResponseData{
		CustomID: modalID,
		Title:    "Backfill scores back to (UTC)",
		Components: []discordgo.MessageComponent{
			textField(fieldDay, "Day", strconv.Itoa(d), 2),
			textField(fieldMonth, "Month", strconv.Itoa(int(m)), 2),
			textField(fieldYear, "Year", strconv.Itoa(y), 4),
		},
	}
}

// text input values by CustomID, wherever they sit in the modal's rows
func modalValues(components []discordgo.MessageComponent, into map[string]string) {
	for _, c := range components {
		switch c := c.(type) {
		case *discordgo.ActionsRow:
			modalValues(c.Components, into)
		case *discordgo.TextInput:
			into[c.CustomID] = strings.TrimSpace(c.Value)
		}
	}
}

// stage 1 of the date: modal text -> 00:00 UTC unix, rejecting the future
func parseDate(values map[string]string) (dayStart int64, err error) {
	var parts [3]int
	for j, field := range []string{fieldYear, fieldMonth, fieldDay} {
		n, err := strconv.Atoi(values[field])
		if err != nil {
			return 0, fmt.Errorf("%s isn't a number", field)
		}
		parts[j] = n
	}
	dayStart, err = unix.DateToDayStart(parts[0], parts[1], parts[2])
	if err != nil {
		return 0, err
	}
	if dayStart > unix.TodayStart() {
		return 0, errors.New("that day hasn't happened yet")
	}
	return dayStart, nil
}

func reply(s session, i intxn, content string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	})
}

// edits the ephemeral reply the router deferred for the modal submit
func editDeferred(s session, i intxn, content string) error {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
	return err
}

// modal submit -> validate -> start the job. rank is checked here since prefix
// handlers bypass the router's per-command check
func onSubmit(bcp bcpackage) func(s session, i intxn) (bool, error) {
	return func(s session, i intxn) (bool, error) {
		uid := i.Member.User.ID
		if !bcp.Auth.CanUse(uid, metadata.MinTrustLevel) {
			return true, editDeferred(s, i, "Sorry, you can't use that command.")
		}
		if !bcp.Tracking.IsTracked(i.GuildID, i.ChannelID) {
			return true, editDeferred(s, i, "This channel isn't tracked. Use /track first.")
		}
		values := make(map[string]string, 3)
		modalValues(i.ModalSubmitData().Components, values)
		dayStart, err := parseDate(values)
		if err != nil {
			return true, editDeferred(s, i, "❌ "+err.Error())
		}

		job, err := Start(s, bcp, i.GuildID, i.ChannelID, dayStart)
		if err != nil {
			return true, editDeferred(s, i, "❌ "+err.Error())
		}
		return true, editDeferred(s, i, fmt.Sprintf("Backfill started: https://discord.com/channels/%s/%s/%s", i.GuildID, i.ChannelID, job.progressID))
	}
}

// anyone with the command's rank may cancel, not just whoever started it
func onCancel(bcp bcpackage) func(s session, i intxn) (bool, error) {
	return func(s session, i intxn) (bool, error) {
		if !bcp.Auth.CanUse(i.Member.User.ID, metadata.MinTrustLevel) {
			return true, nil
		}
		id := strings.TrimPrefix(i.MessageComponentData().CustomID, cancelPrefix)
		if job, ok := lookup(id); ok {
			job.cancel()
			return true, nil
		}
		// not running here (finished, or from before a restart)
		content := "-# This backfill is no longer running."
		empty := []discordgo.MessageComponent{}
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, Components: &empty})
		return true, err
	}
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	bcp.Components.AddPrefixHandler(modalID, onSubmit(bcp))
	bcp.Components.AddPrefixHandler(cancelPrefix, onCancel(bcp))

	(*bcp.Handles)["backfill"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "backfill",
			Description: "read old krillion results in this channel back to a date",
		},
		Metadata: metadata,
		Callback: func(s session, i intxn) error {
			if !bcp.Tracking.IsTracked(i.GuildID, i.ChannelID) {
				return reply(s, i, "This channel isn't tracked. Use /track first.")
			}
			if _, busy := running(i.ChannelID); busy {
				return reply(s, i, "A backfill is already running in this channel.")
			}
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: dateModal(),
			})
		},
	}
}
