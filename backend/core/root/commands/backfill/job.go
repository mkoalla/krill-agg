package backfill

import (
	"errors"
	"fmt"
	"krill-agg/root/info/unix"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/scores"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
)

// messages per fetch (discord's max) and the minimum gap between progress edits
const (
	PAGE_SIZE      = 100
	PROGRESS_EVERY = 2 * time.Second
)

const (
	stateRunning int32 = iota
	stateCanceled
	stateCommitting
)

// a score plus when it was posted, so the oldest post per user per day wins
type timedScore struct {
	score  krillion.PlayerScore
	posted time.Time
}

type Job struct {
	id         string
	session    *discordgo.Session
	scores     *scores.ScoreManager
	guildID    string
	channelID  string
	progressID string
	dayStart   time.Time

	state atomic.Int32

	// only touched by the scan goroutine
	found    map[int]map[string]timedScore // krillion day -> user -> oldest score
	counted  int
	scanned  int
	current  time.Time
	reported time.Time
}

// job id -> job, and channel -> job id so only one runs per channel
var (
	registry  sync.Mutex
	jobs      = map[string]*Job{}
	byChannel = map[string]string{}
)

func lookup(id string) (*Job, bool) {
	registry.Lock()
	defer registry.Unlock()
	job, ok := jobs[id]
	return job, ok
}

func running(channelID string) (*Job, bool) {
	registry.Lock()
	defer registry.Unlock()
	job, ok := jobs[byChannel[channelID]]
	return job, ok
}

// scans the channel back through the UTC day containing since (unix seconds,
// floored here so any timestamp works, e.g. from a cron job). posts the
// progress message, then runs in the background
func Start(s *discordgo.Session, bcp bcpackage, guildID, channelID string, since int64) (*Job, error) {
	dayStart := unix.FloorToDayStart(since)
	if dayStart > unix.TodayStart() {
		return nil, errors.New("that day hasn't happened yet")
	}
	job := &Job{
		id:        uuid.NewString(),
		session:   s,
		scores:    bcp.Scores,
		guildID:   guildID,
		channelID: channelID,
		dayStart:  time.Unix(dayStart, 0).UTC(),
		found:     map[int]map[string]timedScore{},
		current:   time.Now(),
	}

	registry.Lock()
	if _, busy := jobs[byChannel[channelID]]; busy {
		registry.Unlock()
		return nil, errors.New("a backfill is already running in this channel")
	}
	jobs[job.id] = job
	byChannel[channelID] = job.id
	registry.Unlock()

	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:    job.progress(),
		Components: job.buttons(),
	})
	if err != nil {
		job.unregister()
		return nil, err
	}
	job.progressID = msg.ID
	go job.run()
	return job, nil
}

func (job *Job) unregister() {
	registry.Lock()
	defer registry.Unlock()
	delete(jobs, job.id)
	if byChannel[job.channelID] == job.id {
		delete(byChannel, job.channelID)
	}
}

func (job *Job) cancel() {
	job.state.CompareAndSwap(stateRunning, stateCanceled)
}

func (job *Job) buttons() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Cancel", Style: discordgo.DangerButton, CustomID: cancelPrefix + job.id},
		}},
	}
}

func (job *Job) progress() string {
	return fmt.Sprintf("### Backfilling back to <t:%d:D>\nScanned %d messages · %d scores counted\n-# currently at <t:%d:f>",
		job.dayStart.Unix(), job.scanned, job.counted, job.current.Unix())
}

// final edit: replaces the progress text and drops the button
func (job *Job) finish(content string) {
	empty := []discordgo.MessageComponent{}
	if _, err := job.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    job.channelID,
		ID:         job.progressID,
		Content:    &content,
		Components: &empty,
	}); err != nil {
		log.Printf("backfill %s: final edit: %v", job.id, err)
	}
}

func (job *Job) report() {
	if time.Since(job.reported) < PROGRESS_EVERY {
		return
	}
	job.reported = time.Now()
	content := job.progress()
	components := job.buttons()
	if _, err := job.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    job.channelID,
		ID:         job.progressID,
		Content:    &content,
		Components: &components,
	}); err != nil {
		log.Printf("backfill %s: progress edit: %v", job.id, err)
	}
}

// keeps the oldest valid score per user per krillion day. invalid ones are
// dropped silently: no reactions or replies on old messages
func (job *Job) consider(m *discordgo.Message) {
	if m.Author == nil || m.Author.Bot {
		return
	}
	p, err := krillion.FromMessage(m.Content)
	if err != nil {
		return
	}
	users, ok := job.found[p.Day]
	if !ok {
		users = map[string]timedScore{}
		job.found[p.Day] = users
	}
	existing, ok := users[m.Author.ID]
	if !ok {
		job.counted++
	} else if !m.Timestamp.Before(existing.posted) {
		return
	}
	users[m.Author.ID] = timedScore{score: p, posted: m.Timestamp}
}

// pages backwards from the progress message until a message older than
// dayStart shows up (so the whole start day is read) or the channel ends
func (job *Job) run() {
	defer job.unregister()

	before := job.progressID
	for {
		if job.state.Load() == stateCanceled {
			job.finish("-# Backfill canceled, nothing was saved.")
			return
		}
		page, err := job.session.ChannelMessages(job.channelID, PAGE_SIZE, before, "", "")
		if err != nil {
			job.finish(fmt.Sprintf("❌ Backfill stopped, nothing was saved: %v", err))
			return
		}
		done := len(page) < PAGE_SIZE
		for _, m := range page {
			if m.Timestamp.Before(job.dayStart) {
				done = true
				break
			}
			job.scanned++
			job.current = m.Timestamp
			job.consider(m)
		}
		if len(page) > 0 {
			before = page[len(page)-1].ID
		}
		if done {
			break
		}
		job.report()
	}

	// past this point a cancel no longer applies
	if !job.state.CompareAndSwap(stateRunning, stateCommitting) {
		job.finish("-# Backfill canceled, nothing was saved.")
		return
	}
	if err := job.commit(); err != nil {
		job.finish(fmt.Sprintf("❌ Backfill read %d messages but couldn't save: %v", job.scanned, err))
		return
	}
	job.finish(fmt.Sprintf("✅ Backfill done back to <t:%d:D>: scanned %d messages, saved %d scores across %d days.",
		job.dayStart.Unix(), job.scanned, job.counted, len(job.found)))
}

// the scan read the true oldest post, so it replaces whatever is saved
func (job *Job) commit() error {
	entries := make([]scores.Entry, 0, job.counted)
	for _, users := range job.found {
		for uid, ts := range users {
			entries = append(entries, scores.Entry{UserID: uid, Score: ts.score})
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return job.scores.SetMany(job.guildID, entries)
}
