package components

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// handles a component click or modal submit by its CustomID prefix, before
// the per-user groups get a look. for things that aren't tied to one user
// (rank-gated buttons, leftovers from a previous run). handled=false falls
// through to normal routing
type PrefixCallback = func(s *discordgo.Session, i *discordgo.InteractionCreate) (handled bool, err error)

// claims clicks whose CustomID starts with prefix
func (man *ComponentCallbackManager) AddPrefixHandler(prefix string, f PrefixCallback) {
	man.mutex.Lock()
	defer man.mutex.Unlock()
	man.prefixes[prefix] = f
}

func (man *ComponentCallbackManager) TryPrefix(s *discordgo.Session, i *discordgo.InteractionCreate) (handled bool, err error) {
	var cid string
	switch i.Type {
	case discordgo.InteractionMessageComponent:
		cid = i.MessageComponentData().CustomID
	case discordgo.InteractionModalSubmit:
		cid = i.ModalSubmitData().CustomID
	default:
		return false, nil
	}
	man.mutex.RLock()
	var match PrefixCallback
	for prefix, f := range man.prefixes {
		if strings.HasPrefix(cid, prefix) {
			match = f
			break
		}
	}
	man.mutex.RUnlock()
	if match == nil {
		return false, nil
	}
	return match(s, i)
}
