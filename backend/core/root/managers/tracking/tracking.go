package tracking

import (
	"krill-agg/root/datas/trackdb"
	"maps"
	"slices"
	"sync"
)

type guildID = string
type channelID = string

// guild -> tracked channels and its main channel (where daily posts go),
// persisted in trackdb and mirrored in memory since every message checks
// IsTracked. a guild with no channels has no key. memory only changes once
// the file has
type TrackingManager struct {
	mu      sync.RWMutex
	db      *trackdb.TrackDB
	tracked map[guildID][]channelID
	mains   map[guildID]channelID
}

func New(path string) (*TrackingManager, error) {
	db, err := trackdb.New(path)
	if err != nil {
		return nil, err
	}
	tracked, mains, err := db.Load()
	if err != nil {
		db.Close() //nolint
		return nil, err
	}
	return &TrackingManager{db: db, tracked: tracked, mains: mains}, nil
}

func (man *TrackingManager) Close() error { return man.db.Close() }

func (man *TrackingManager) IsTracked(guild guildID, channel channelID) bool {
	man.mu.RLock()
	defer man.mu.RUnlock()
	return slices.Contains(man.tracked[guild], channel)
}

// copy of the guild's tracked channels, in the order they were added
func (man *TrackingManager) Channels(guild guildID) []channelID {
	man.mu.RLock()
	defer man.mu.RUnlock()
	return slices.Clone(man.tracked[guild])
}

func (man *TrackingManager) Main(guild guildID) (channelID, bool) {
	man.mu.RLock()
	defer man.mu.RUnlock()
	c, ok := man.mains[guild]
	return c, ok
}

// copy of every guild's main channel
func (man *TrackingManager) Mains() map[guildID]channelID {
	man.mu.RLock()
	defer man.mu.RUnlock()
	return maps.Clone(man.mains)
}

// adds channels not already tracked; returns how many were new. a guild that
// had nothing tracked gets its first new channel as main (returned in main)
func (man *TrackingManager) Track(guild guildID, channels ...channelID) (added int, main channelID, err error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	tracked := man.tracked[guild]
	var fresh []channelID
	for _, c := range channels {
		if !slices.Contains(tracked, c) && !slices.Contains(fresh, c) {
			fresh = append(fresh, c)
		}
	}
	if len(fresh) == 0 {
		return 0, "", nil
	}
	if len(tracked) == 0 {
		main = fresh[0]
	}
	if err := man.db.Track(guild, fresh, main); err != nil {
		return 0, "", err
	}
	man.tracked[guild] = append(tracked, fresh...)
	if main != "" {
		man.mains[guild] = main
	}
	return len(fresh), main, nil
}

// removes channels; returns how many were tracked, and whether the main
// channel was among them (it's cleared with them)
func (man *TrackingManager) Untrack(guild guildID, channels ...channelID) (removed int, lostMain bool, err error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	tracked := man.tracked[guild]
	var gone []channelID
	for _, c := range tracked {
		if slices.Contains(channels, c) {
			gone = append(gone, c)
		}
	}
	if len(gone) == 0 {
		return 0, false, nil
	}
	if err := man.db.Untrack(guild, gone); err != nil {
		return 0, false, err
	}
	man.setTracked(guild, slices.DeleteFunc(slices.Clone(tracked), func(c channelID) bool {
		return slices.Contains(gone, c)
	}))
	if main, ok := man.mains[guild]; ok && slices.Contains(gone, main) {
		delete(man.mains, guild)
		lostMain = true
	}
	return len(gone), lostMain, nil
}

// drops every tracked channel in the guild, and its main channel; returns how many there were
func (man *TrackingManager) UntrackAll(guild guildID) (removed int, err error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	removed = len(man.tracked[guild])
	if removed == 0 {
		return 0, nil
	}
	if err := man.db.UntrackAll(guild); err != nil {
		return 0, err
	}
	delete(man.tracked, guild)
	delete(man.mains, guild)
	return removed, nil
}

// makes channel the guild's main channel, overwriting any other, and tracks
// it if it wasn't. old is the previous main ("" if none)
func (man *TrackingManager) SetMain(guild guildID, channel channelID) (old channelID, newlyTracked bool, err error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	if err := man.db.SetMain(guild, channel); err != nil {
		return "", false, err
	}
	old = man.mains[guild]
	if !slices.Contains(man.tracked[guild], channel) {
		man.tracked[guild] = append(man.tracked[guild], channel)
		newlyTracked = true
	}
	man.mains[guild] = channel
	return old, newlyTracked, nil
}

// caller holds the lock; empty lists drop the key
func (man *TrackingManager) setTracked(guild guildID, tracked []channelID) {
	if len(tracked) == 0 {
		delete(man.tracked, guild)
		return
	}
	man.tracked[guild] = tracked
}
