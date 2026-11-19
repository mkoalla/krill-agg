package scores

import "sync"

// a guild's all-time scores, summed: enough for their mean and spread without
// reading them all again. whole numbers, so adding and taking away never drifts
type Moments struct {
	N     int64 // games
	Sum   int64 // of their scores
	SumSq int64 // of their scores squared
}

func (m *Moments) add(score int, sign int64) {
	s := int64(score)
	m.N += sign
	m.Sum += sign * s
	m.SumSq += sign * s * s
}

// guild -> its running Moments. a guild is read in full the first time it's
// asked for, then kept up to date by every write. mu is held across each
// write and its update (and across that first read), so a write can't land
// between them and be counted twice or not at all
type moments struct {
	mu     sync.Mutex
	guilds map[guildID]*Moments
}

// changes a guild's running totals, if they're being kept yet
func (ms *moments) addLocked(guild guildID, score int, sign int64) {
	if m, ok := ms.guilds[guild]; ok {
		m.add(score, sign)
	}
}

// forgets a guild's totals; the next ask reads it in full again. for bulk
// writes, where reading each old score first costs more than that one read
func (ms *moments) forgetLocked(guild guildID) {
	delete(ms.guilds, guild)
}

// every score in a guild, all-time, summed. read in full only the first time
func (man *ScoreManager) Moments(guild guildID) (Moments, error) {
	man.moments.mu.Lock()
	defer man.moments.mu.Unlock()
	if m, ok := man.moments.guilds[guild]; ok {
		return *m, nil
	}
	entries, err := man.Guild(guild, nil, 1)
	if err != nil {
		return Moments{}, err
	}
	m := &Moments{}
	for _, e := range entries {
		m.add(e.Score.Score, 1)
	}
	if man.moments.guilds == nil {
		man.moments.guilds = map[guildID]*Moments{}
	}
	man.moments.guilds[guild] = m
	return *m, nil
}
