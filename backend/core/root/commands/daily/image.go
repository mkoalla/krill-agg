package daily

import (
	"context"
	"errors"
	"fmt"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/info/theme"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/distributions"
	"krill-agg/root/types"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// how long a posted card can stand in for a fresh one
const REUSE_FOR = 30 * time.Minute

// the last card posted per guild, by its discord attachment url so reusing it
// needs no upload at all
type cachedImage struct {
	day int
	at  time.Time
	url string
}

// the one render underway in a guild. link (to the reply carrying it) is set
// before posted closes, and stays empty if that reply never went out. finished
// closes once the render is over and its card (if any) remembered
type inflight struct {
	posted   chan struct{}
	finished chan struct{}
	once     sync.Once // publish
	ended    sync.Once // done
	link     string
}

func (r *inflight) publish(link string) {
	r.once.Do(func() {
		r.link = link
		close(r.posted)
	})
}

var (
	imageMu sync.Mutex
	images  = map[string]cachedImage{}
	renders = map[string]*inflight{}
)

func cachedLocked(guild string, day int, now time.Time) (cachedImage, bool) {
	c, ok := images[guild]
	if !ok || c.day != day || now.Sub(c.at) >= REUSE_FOR {
		return cachedImage{}, false
	}
	return c, true
}

func cached(guild string, day int, now time.Time) (cachedImage, bool) {
	imageMu.Lock()
	defer imageMu.Unlock()
	return cachedLocked(guild, day, now)
}

func remember(guild string, c cachedImage) {
	imageMu.Lock()
	defer imageMu.Unlock()
	images[guild] = c
}

// the outcome of claim, exactly one of:
//   - cached: a recent card to reuse
//   - follow: another call is rendering for this guild; link to its reply
//   - lead: this call renders, and must call done when finished
//
// or, from claimPast, a past day's card: cached (its link still works), or
// past (upload the card in dir, drawing it first if card is empty, and note
// the new link)
type ticket struct {
	cached *cachedImage
	follow *inflight
	lead   *inflight
	guild  string
	past   bool
	dir    string // past: data/dailygen/<guild>/<day>
	card   string // past: the card already on disk, if any
}

// next to a past day's card: the discord link it was last posted at
const LINK_FILE = "link"

// discord's attachment links expire; this only asks for the headers
var linkCheck = &http.Client{Timeout: 3 * time.Second}

func linkAlive(url string) bool {
	resp, err := linkCheck.Head(url)
	if err != nil {
		return false
	}
	resp.Body.Close() //nolint
	return resp.StatusCode == http.StatusOK
}

// a past day's card (dir is data/dailygen/<guild>/<day>): its last link if
// that still works, else the card on disk to upload again, else nothing yet
// (draw it). outside the one-render-per-guild claim, which is today's. a
// finished day's card doesn't change, so it's drawn once and only re-uploaded
func claimPast(dir string, day int) *ticket {
	linkPath := filepath.Join(dir, LINK_FILE)
	if raw, err := os.ReadFile(linkPath); err == nil {
		url := strings.TrimSpace(string(raw))
		if info, err := os.Stat(linkPath); err == nil && url != "" && linkAlive(url) {
			return &ticket{cached: &cachedImage{day: day, at: info.ModTime(), url: url}}
		}
	}
	t := &ticket{past: true, dir: dir}
	card := filepath.Join(dir, fmt.Sprintf("daily_%d.webp", day))
	if _, err := os.Stat(card); err == nil {
		t.card = card
	}
	return t
}

// notes where a past day's card now lives, replacing the old link
func writeLink(dir, url string) error {
	tmp, err := os.CreateTemp(dir, ".link-")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(url + "\n"); err != nil {
		tmp.Close()           //nolint
		os.Remove(tmp.Name()) //nolint
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name()) //nolint
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, LINK_FILE))
}

// one render per guild at a time. a cache hit wins (unless fresh); otherwise
// joining a render already underway beats starting a second one. all decided
// under one lock, so two simultaneous calls can't both lead
func claim(guild string, day int, now time.Time, fresh bool) ticket {
	imageMu.Lock()
	defer imageMu.Unlock()
	if !fresh {
		if c, ok := cachedLocked(guild, day, now); ok {
			return ticket{cached: &c}
		}
	}
	if r, ok := renders[guild]; ok {
		return ticket{follow: r}
	}
	r := &inflight{posted: make(chan struct{}), finished: make(chan struct{})}
	renders[guild] = r
	return ticket{lead: r, guild: guild}
}

// frees the guild for the next render (repeat calls are no-ops); followers still waiting on a reply
// that never went out are released with no link and claim again
func (t ticket) done() {
	if t.lead == nil {
		return
	}
	t.lead.ended.Do(func() {
		imageMu.Lock()
		if renders[t.guild] == t.lead {
			delete(renders, t.guild)
		}
		imageMu.Unlock()
		t.lead.publish("")
		close(t.lead.finished)
	})
}

// guild is the fallback for when discord leaves the message's own guild off
func jumpLink(guild string, msg *discordgo.Message) string {
	if msg.GuildID != "" {
		guild = msg.GuildID
	}
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guild, msg.ChannelID, msg.ID)
}

// the cdn url discord gave the uploaded card. the embed's image is rewritten
// from attachment:// to it; the attachment list is the fallback
func postedURL(msg *discordgo.Message) string {
	for _, e := range msg.Embeds {
		if e.Image != nil && strings.HasPrefix(e.Image.URL, "http") {
			return e.Image.URL
		}
	}
	if len(msg.Attachments) > 0 {
		return msg.Attachments[0].URL
	}
	return ""
}

// cached first; the api only when the state is missing it
func getGuild(s *discordgo.Session, guild string) *discordgo.Guild {
	if g, err := s.State.Guild(guild); err == nil && g.Name != "" {
		return g
	}
	if g, err := s.Guild(guild); err == nil {
		return g
	}
	return &discordgo.Guild{ID: guild}
}

// how long the bot waits on dailygen for one card
const RENDER_TIMEOUT = 5 * time.Second

// fetches avatars and has dailygen render the card; returns the card's path on
// disk. a day nobody played still gets a card (no average), as does one
// krillion has no distribution for (a flat line, no percentile)
func GenerateImage(s *discordgo.Session, bcp *types.BotCommandPackage, snap *Snapshot) (string, error) {
	ids := make([]string, len(snap.Entries))
	users := make([]dailygen.User, len(snap.Entries))
	total := 0
	for j, e := range snap.Entries {
		ids[j] = e.UserID
		users[j] = dailygen.User{Score: e.Score.Score}
		total += e.Score.Score
	}
	req := dailygen.Request{
		Guild:   snap.Guild,
		Day:     snap.Day,
		Dist:    []float64{},
		Users:   users,
		MaxPush: dailygen.MAX_PUSH,
	}
	hasDist := distributions.HasDist(snap.Dist)
	if hasDist {
		req.Dist = snap.Dist.Dist
		req.Sample = snap.Dist.Sample
	}
	if len(snap.Entries) > 0 {
		mean := float64(total) / float64(len(snap.Entries))
		req.Mean = &mean
		if hasDist {
			pct := snap.Dist.Percentile(mean)
			req.Percentile = &pct
		}
	}

	g := getGuild(s, snap.Guild)
	req.Title = g.Name
	root := filepath.Join(bcp.DataDir, dailygen.DIR)
	images, icon, _, err := SaveAvatars(s, filepath.Join(root, snap.Guild, strconv.Itoa(snap.Day)), g, ids)
	if err != nil {
		return "", fmt.Errorf("saving avatars: %v", err)
	}
	defer os.RemoveAll(images) //nolint // only needed while drawing
	req.Images = filepath.Base(images)
	req.Icon = icon
	// the budget is just for dailygen; the discord lookups above keep their own
	ctx, cancel := context.WithTimeout(context.Background(), RENDER_TIMEOUT)
	defer cancel()
	path, err := dailygen.Render(ctx, req)
	if errors.Is(err, context.DeadlineExceeded) {
		return "", fmt.Errorf("the renderer took longer than %v", RENDER_TIMEOUT)
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(root, path), nil
}

func imageEmbed(description string, url string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{Description: description, Color: theme.ACCENT}
	if url != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: url}
	}
	return embed
}

// the zero value is what /forcedaily gets. not command options: only code sets them
type ImageOptions struct {
	// skip the cache and render anew (a scheduled post wants today's latest).
	// still one render per guild: one already underway is linked to instead
	Fresh bool
	// name today's winner(s) under the heading; the day isn't over for a
	// mid-day /forcedaily, so only the scheduled post does
	Winners bool
}

// claims this guild's render for the interaction, editing its reply to point
// at the one underway if there is one. lead is nil when that's all there is to
// do; otherwise the caller goes on to the report and withImage, then lead.done()
func claimFor(s *discordgo.Session, i *discordgo.InteractionCreate, day int, opts ImageOptions) (*ticket, error) {
	for {
		t := claim(i.GuildID, day, time.Now(), opts.Fresh)
		if t.follow == nil {
			return &t, nil
		}
		// waits only for the leader's first reply (its board), not the render
		<-t.follow.posted
		if t.follow.link == "" {
			continue // it failed before replying; try again, maybe as the leader
		}
		content := "-# Already generating: " + t.follow.link
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return nil, err
	}
}

// waits out any render underway in the guild, then claims it. for posts with
// no reply to link from; with fresh it always ends up leading
func claimWait(guild string, day int, opts ImageOptions) ticket {
	for {
		t := claim(guild, day, time.Now(), opts.Fresh)
		if t.follow == nil {
			return t
		}
		<-t.follow.finished
	}
}

// puts the whole message (content and embeds) wherever it's going; each call
// replaces the last. files are attached on top
type sender = func(content string, embeds []*discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error)

var noPings = &discordgo.MessageAllowedMentions{}

// edits the interaction's (deferred) reply
func replySender(s *discordgo.Session, i *discordgo.InteractionCreate) sender {
	return func(content string, embeds []*discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error) {
		return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content, Embeds: &embeds, Files: files, AllowedMentions: noPings})
	}
}

// a plain message in channel: sent on the first call, edited after
func channelSender(s *discordgo.Session, channel string) sender {
	var sent *discordgo.Message
	return func(content string, embeds []*discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error) {
		if sent == nil {
			msg, err := s.ChannelMessageSendComplex(channel, &discordgo.MessageSend{Content: content, Embeds: embeds, Files: files, AllowedMentions: noPings})
			if err == nil {
				sent = msg
			}
			return msg, err
		}
		return s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: sent.ID, Channel: sent.ChannelID, Content: &content, Embeds: &embeds, Files: files, AllowedMentions: noPings,
		})
	}
}

// posts the report with the card as a second embed in the same message: a
// cached card straight away, or a placeholder (so the board never waits) and
// then an edit that attaches the card. t comes from claimFor or claimWait
// (today's card), or claimPast (a past day's, from /stats)
func withImage(s *discordgo.Session, post sender, bcp *types.BotCommandPackage, snap *Snapshot, report []*discordgo.MessageEmbed, t *ticket, opts ImageOptions) (*discordgo.Message, error) {
	send := func(image *discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error) {
		embeds := append(append([]*discordgo.MessageEmbed{}, report...), image)
		return post(Content(snap, opts.Winners), embeds, files)
	}

	if t.cached != nil {
		return send(imageEmbed(fmt.Sprintf("-# Image reused from <t:%d:R>", t.cached.at.Unix()), t.cached.url), nil)
	}
	msg, err := send(imageEmbed("-# generating image...", ""), nil)
	if err != nil {
		return nil, err
	}
	if t.lead != nil {
		t.lead.publish(jumpLink(snap.Guild, msg))
	}
	fail := func(reason error) (*discordgo.Message, error) {
		_, errEdit := send(imageEmbed("-# couldn't produce image: "+reason.Error(), ""), nil)
		return msg, errors.Join(fmt.Errorf("daily image: %v", reason), errEdit)
	}

	path := t.card // a past day's card already drawn: just upload it again
	if path == "" {
		if path, err = GenerateImage(s, bcp, snap); err != nil {
			return fail(err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(err)
	}
	defer f.Close() //nolint

	name := filepath.Base(path)
	final, err := send(imageEmbed("", "attachment://"+name), []*discordgo.File{{Name: name, ContentType: "image/webp", Reader: f}})
	if err != nil {
		return fail(err)
	}
	url := postedURL(final)
	if url == "" {
		return final, errors.New("daily image: posted, but discord returned no url to reuse")
	}
	if t.past {
		if err := writeLink(t.dir, url); err != nil {
			return final, fmt.Errorf("daily image: saving its link: %v", err)
		}
		return final, nil
	}
	remember(snap.Guild, cachedImage{day: snap.Day, at: time.Now(), url: url})
	return final, nil
}

// today's report as a plain message in channel, for scheduled posts (no
// interaction behind it). never cached: it waits out any render underway in
// the guild, then renders its own. returns a link to the message
func PostDaily(s *discordgo.Session, bcp *types.BotCommandPackage, channel string) (string, error) {
	ch, err := s.State.Channel(channel)
	if err != nil {
		if ch, err = s.Channel(channel); err != nil {
			return "", fmt.Errorf("channel %s: %v", channel, err)
		}
	}
	g := ch.GuildID
	if g == "" {
		return "", fmt.Errorf("channel %s isn't in a guild", channel)
	}
	day := krillion.Today()
	opts := ImageOptions{Fresh: true, Winners: true}
	t := claimWait(g, day, opts)
	defer t.done()
	embeds, snap, err := Report(bcp, g, day)
	if err != nil {
		return "", err
	}
	msg, err := withImage(s, channelSender(s, channel), bcp, snap, embeds, &t, opts)
	err = errors.Join(err, refetchBoards(bcp, g))
	if msg == nil {
		return "", err
	}
	return jumpLink(g, msg), err
}
