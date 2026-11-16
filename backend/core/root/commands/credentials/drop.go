package credentials

import (
	"fmt"
	"poll-bot/root/managers/components"

	"github.com/bwmarrin/discordgo"
)

func iModalSubmitDrop(s session, state *iState) (bool, error) {
	i := state.interaction
	web := state.manager

	resp := "I'm validating this information..."
	msg, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &resp,
	})
	if err != nil {
		return false, err
	}
	state.message = msg

	data := i.ModalSubmitData()
	idField := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	err = web.DropUser(idField)

	var message string
	if err != nil {
		message = fmt.Sprintf("<@%s>, I couldn't drop this user: %v", i.Member.User.ID, err)
	} else {
		message = fmt.Sprintf("<@%s>, `%s` has been revoked, if they exist.", i.Member.User.ID, idField)
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &message,
	})
	return true, err
}
func iModalCloseDrop(s session, state *iState) (bool, error) {
	return true, nil
}

func createCredentialDropSession(bcp bcpackage, man *webman, s session, i intxn) error {
	//default inits
	thisState := iState{
		interaction: i,
		manager:     man,
	}
	inject := newInjector(s, &thisState)
	//busy atomic in manager should help w debounce
	return bcp.Components.Register("cred-drop", components.NewComponentCallbacks(i.Member.User.ID, map[string]components.Callback{
		"cred-drop-modal-submit": inject(iModalSubmitDrop),
		"cred-drop-close":        inject(iModalCloseDrop),
	}))
}
func cmd_drop_credentials(s session, i intxn, bcp bcpackage, web *webman) error {
	if err := createCredentialDropSession(bcp, web, s, i); err != nil {
		return err
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "cred-drop-modal-submit",
			Title:    "Revoke Credentials",
			Components: makeFields([]field{
				{"Id", "cred-drop-id", ""},
			}),
		}})
	return err
}
