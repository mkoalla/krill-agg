package tracking

import (
	fd "krill-agg/root/datas/filedict"
	"slices"
	"strconv"
	"sync"
)

type guildID = string
type channelID = string

// guild -> tracked channels, persisted with a filedict. a guild with no channels has no key
type TrackingManager struct {
	mu   sync.Mutex // makes each read-modify-write atomic
	data *fd.FileDict[guildID, []channelID]
}

func validate(guild *guildID, channels *[]channelID) bool {
	if _, err := strconv.ParseUint(*guild, 10, 64); err != nil {
		return false
	}
	for _, c := range *channels {
		if _, err := strconv.ParseUint(c, 10, 64); err != nil {
			return false
		}
	}
	return true
}

func New(path string) (*TrackingManager, error) {
	table, err := fd.New(path, validate)
	if err != nil {
		return nil, err
	}
	return &TrackingManager{data: table}, nil
}

func (man *TrackingManager) Close() error { return man.data.Close() }

func (man *TrackingManager) IsTracked(guild guildID, channel channelID) bool {
	tracked, _ := man.data.Get(guild)
	return slices.Contains(tracked, channel)
}

// copy of the guild's tracked channels, in the order they were added
func (man *TrackingManager) Channels(guild guildID) []channelID {
	tracked, _ := man.data.Get(guild)
	return slices.Clone(tracked)
}

// adds channels not already tracked; returns how many were new
func (man *TrackingManager) Track(guild guildID, channels ...channelID) (added int, err error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	tracked := man.Channels(guild)
	for _, c := range channels {
		if !slices.Contains(tracked, c) {
			tracked = append(tracked, c)
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}
	return added, man.save(guild, tracked)
}

// removes channels; returns how many were tracked
func (man *TrackingManager) Untrack(guild guildID, channels ...channelID) (removed int, err error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	tracked := man.Channels(guild)
	before := len(tracked)
	tracked = slices.DeleteFunc(tracked, func(c channelID) bool {
		return slices.Contains(channels, c)
	})
	removed = before - len(tracked)
	if removed == 0 {
		return 0, nil
	}
	return removed, man.save(guild, tracked)
}

// drops every tracked channel in the guild; returns how many there were
func (man *TrackingManager) UntrackAll(guild guildID) (removed int, err error) {
	man.mu.Lock()
	defer man.mu.Unlock()
	removed = len(man.Channels(guild))
	if removed == 0 {
		return 0, nil
	}
	return removed, man.save(guild, nil)
}

// caller holds the lock. writes only this guild's row; empty lists drop it
func (man *TrackingManager) save(guild guildID, tracked []channelID) error {
	if len(tracked) == 0 {
		return man.data.DropSync(guild)
	}
	return man.data.SetSync(guild, tracked)
}
