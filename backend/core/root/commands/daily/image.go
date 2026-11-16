package daily

import (
	"errors"
	"fmt"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/types"
	"os"
	"path/filepath"
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
// before posted closes, and stays empty if that reply never went out
type inflight struct {
	posted chan struct{}
	once   sync.Once
	link   string
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
type ticket struct {
	cached *cachedImage
	follow *inflight
	lead   *inflight
	guild  string
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
	r := &inflight{posted: make(chan struct{})}
	renders[guild] = r
	return ticket{lead: r, guild: guild}
}

// frees the guild for the next render; followers still waiting on a reply
// that never went out are released with no link and claim again
func (t ticket) done() {
	if t.lead == nil {
		return
	}
	imageMu.Lock()
	if renders[t.guild] == t.lead {
		delete(renders, t.guild)
	}
	imageMu.Unlock()
	t.lead.publish("")
}

func jumpLink(guild, channel string, msg *discordgo.Message) string {
	if msg.ChannelID != "" {
		channel = msg.ChannelID
	}
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guild, channel, msg.ID)
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

// fetches avatars and has dailygen render the card; returns the png's path on disk
func GenerateImage(s *discordgo.Session, bcp *types.BotCommandPackage, snap *Snapshot) (string, error) {
	if len(snap.Entries) == 0 {
		return "", errors.New("no players yet")
	}
	ids := make([]string, len(snap.Entries))
	users := make([]dailygen.User, len(snap.Entries))
	total := 0
	for j, e := range snap.Entries {
		ids[j] = e.UserID
		users[j] = dailygen.User{Score: e.Score.Score}
		total += e.Score.Score
	}
	mean := float64(total) / float64(len(snap.Entries))

	g := getGuild(s, snap.Guild)
	dir := filepath.Join(bcp.DataDir, dailygen.DIR)
	icon, _, err := SaveAvatars(s, dir, g, ids)
	if err != nil {
		return "", fmt.Errorf("saving avatars: %v", err)
	}
	path, err := dailygen.Render(dailygen.Request{
		Guild:      snap.Guild,
		Title:      g.Name,
		Icon:       icon,
		Day:        snap.Day,
		Mean:       mean,
		Dist:       snap.Dist.Dist,
		Percentile: snap.Dist.Percentile(mean),
		Users:      users,
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, path), nil
}

func imageEmbed(description string, url string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{Description: description}
	if url != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: url}
	}
	return embed
}

// the zero value reuses a recent card. not a command option: only code sets it
type ImageOptions struct {
	// skip the cache and render anew (a scheduled post wants today's latest).
	// still one render per guild: one already underway is linked to instead
	Fresh bool
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

// posts the report with the card as a second embed in the same message: a
// cached card straight away, or a placeholder (so the board never waits) and
// then an edit that attaches the png. t comes from claimFor
func withImage(s *discordgo.Session, i *discordgo.InteractionCreate, bcp *types.BotCommandPackage, snap *Snapshot, report []*discordgo.MessageEmbed, t *ticket) error {
	noPings := &discordgo.MessageAllowedMentions{}
	send := func(image *discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error) {
		embeds := append(append([]*discordgo.MessageEmbed{}, report...), image)
		heading := Heading(snap.Day)
		return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &heading, Embeds: &embeds, Files: files, AllowedMentions: noPings})
	}

	if t.cached != nil {
		_, err := send(imageEmbed(fmt.Sprintf("-# Image reused from <t:%d:R>", t.cached.at.Unix()), t.cached.url), nil)
		return err
	}
	msg, err := send(imageEmbed("-# generating image...", ""), nil)
	if err != nil {
		return err
	}
	t.lead.publish(jumpLink(snap.Guild, i.ChannelID, msg))
	fail := func(reason error) error {
		_, errEdit := send(imageEmbed("-# couldn't produce image: "+reason.Error(), ""), nil)
		return errors.Join(fmt.Errorf("daily image: %v", reason), errEdit)
	}

	path, err := GenerateImage(s, bcp, snap)
	if err != nil {
		return fail(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(err)
	}
	defer f.Close() //nolint

	name := filepath.Base(path)
	msg, err = send(imageEmbed("", "attachment://"+name), []*discordgo.File{{Name: name, ContentType: "image/png", Reader: f}})
	if err != nil {
		return fail(err)
	}
	url := postedURL(msg)
	if url == "" {
		return errors.New("daily image: posted, but discord returned no url to reuse")
	}
	remember(snap.Guild, cachedImage{day: snap.Day, at: time.Now(), url: url})
	return nil
}
