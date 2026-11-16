package components

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// handles a component click nothing is registered for, e.g. a button left
// over from a previous run. handled=false falls through to normal routing
type OrphanCallback = func(s *discordgo.Session, i *discordgo.InteractionCreate) (handled bool, err error)

// claims clicks whose CustomID starts with prefix
func (man *ComponentCallbackManager) AddOrphanHandler(prefix string, f OrphanCallback) {
	man.mutex.Lock()
	defer man.mutex.Unlock()
	man.orphans[prefix] = f
}

func (man *ComponentCallbackManager) TryOrphan(s *discordgo.Session, i *discordgo.InteractionCreate) (handled bool, err error) {
	cid := i.MessageComponentData().CustomID
	man.mutex.RLock()
	var match OrphanCallback
	for prefix, f := range man.orphans {
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
