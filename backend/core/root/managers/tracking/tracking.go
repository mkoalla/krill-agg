package tracking

import (
	"slices"
	"sync"
)

type guildID = string
type channelID = string

// in-memory guild -> tracked channels. a guild with no channels has no key
type TrackingManager struct {
	mu       sync.RWMutex
	channels map[guildID][]channelID
}

func New() *TrackingManager {
	return &TrackingManager{channels: make(map[guildID][]channelID)}
}

func (man *TrackingManager) IsTracked(guild guildID, channel channelID) bool {
	man.mu.RLock()
	defer man.mu.RUnlock()
	return slices.Contains(man.channels[guild], channel)
}

// copy of the guild's tracked channels, in the order they were added
func (man *TrackingManager) Channels(guild guildID) []channelID {
	man.mu.RLock()
	defer man.mu.RUnlock()
	return slices.Clone(man.channels[guild])
}

// adds channels not already tracked; returns how many were new
func (man *TrackingManager) Track(guild guildID, channels ...channelID) (added int) {
	man.mu.Lock()
	defer man.mu.Unlock()
	tracked := man.channels[guild]
	for _, c := range channels {
		if !slices.Contains(tracked, c) {
			tracked = append(tracked, c)
			added++
		}
	}
	if len(tracked) > 0 {
		man.channels[guild] = tracked
	}
	return
}

// removes channels; returns how many were tracked
func (man *TrackingManager) Untrack(guild guildID, channels ...channelID) (removed int) {
	man.mu.Lock()
	defer man.mu.Unlock()
	before := len(man.channels[guild])
	tracked := slices.DeleteFunc(man.channels[guild], func(c channelID) bool {
		return slices.Contains(channels, c)
	})
	man.prune(guild, tracked)
	return before - len(tracked)
}

// drops every tracked channel in the guild; returns how many there were
func (man *TrackingManager) UntrackAll(guild guildID) (removed int) {
	man.mu.Lock()
	defer man.mu.Unlock()
	removed = len(man.channels[guild])
	delete(man.channels, guild)
	return
}

// caller holds the lock
func (man *TrackingManager) prune(guild guildID, tracked []channelID) {
	if len(tracked) == 0 {
		delete(man.channels, guild)
		return
	}
	man.channels[guild] = tracked
}
