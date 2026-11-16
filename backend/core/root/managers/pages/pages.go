// Package pages is the paging-only cousin of components: a command hands over
// a list of page builders and gets a message with back/forward buttons that flip
// through them. No cancel and no timeout, it's just an infographic.
//
// Only the user who ran the command can flip it. Like the update prompts, each
// user has an epoch (their newest book is the only live one) and buttons carry
// the run ID, so books from before a restart or superseded by a newer one are
// recognized and closed out instead of answered.
package pages

import (
	"fmt"
	"krill-agg/root/info/run"
	"krill-agg/root/info/theme"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

// button CustomIDs: "pages:<run>:<owner>:<epoch>:<prev|next>"
const PREFIX = "pages:"

const (
	prev = "prev"
	next = "next"
)

// what a page builder is injected with: which page it is, 1-based, of how many
type Info struct {
	Cur int
	Max int
}

// builds one page's embed; called each time the page is shown, so it can read
// live data. Embed is the usual way to fill it in
type Page = func(info Info) (*discordgo.MessageEmbed, error)

// the message generator: the boxed layout most books want, with an optional
// title and icon (thumbnail, top right), the body, and "Page x / y" under it
func Embed(title, icon, body string, info Info) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Color:       theme.ACCENT,
		Title:       title,
		Description: body,
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Page %d / %d", info.Cur, info.Max)},
	}
	if icon != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: icon}
	}
	return embed
}

// one user's live paginated message
type book struct {
	mu    sync.Mutex // one flip at a time
	pages []Page
	cur   int // 0-based
	epoch uint64
}

type Manager struct {
	mu     sync.Mutex
	books  map[string]*book // owner -> their newest book
	epochs map[string]*atomic.Uint64
}

func New() *Manager {
	return &Manager{books: map[string]*book{}, epochs: map[string]*atomic.Uint64{}}
}

// prefix handlers are how clicks reach the manager; call once at startup
func (man *Manager) Register(addPrefix func(prefix string, f func(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error))) {
	addPrefix(PREFIX, man.click)
}

func (man *Manager) nextEpoch(owner string) uint64 {
	e, ok := man.epochs[owner]
	if !ok {
		e = new(atomic.Uint64)
		man.epochs[owner] = e
	}
	return e.Add(1)
}

func customID(owner string, epoch uint64, dir string) string {
	return fmt.Sprintf("%s%s:%s:%d:%s", PREFIX, run.ID, owner, epoch, dir)
}

type clickID struct {
	run   string
	owner string
	epoch uint64
	dir   string
}

func parseID(cid string) (clickID, bool) {
	parts := strings.Split(strings.TrimPrefix(cid, PREFIX), ":")
	if len(parts) != 4 || (parts[3] != prev && parts[3] != next) {
		return clickID{}, false
	}
	epoch, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return clickID{}, false
	}
	return clickID{run: parts[0], owner: parts[1], epoch: epoch, dir: parts[3]}, true
}

// the flip buttons' faces: mudae's own arrow emojis, referenced by id
var (
	emojiPrev = &discordgo.ComponentEmoji{Name: "wleft", ID: "847502744176820256"}
	emojiNext = &discordgo.ComponentEmoji{Name: "wright", ID: "847502746025459792"}
)

// back and forward (blurple), or nothing for a single page
func buttons(owner string, epoch uint64, max int) []discordgo.MessageComponent {
	if max <= 1 {
		return []discordgo.MessageComponent{}
	}
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Emoji: emojiPrev, Style: discordgo.PrimaryButton, CustomID: customID(owner, epoch, prev)},
			discordgo.Button{Emoji: emojiNext, Style: discordgo.PrimaryButton, CustomID: customID(owner, epoch, next)},
		}},
	}
}

// flips wrap around, like mudae's
func turn(cur, max int, dir string) int {
	if dir == prev {
		return (cur - 1 + max) % max
	}
	return (cur + 1) % max
}

// answers the command with the first page. the user who ran it owns the book,
// and their previous one (if any) stops responding
func (man *Manager) Open(s *discordgo.Session, i *discordgo.InteractionCreate, pages []Page) error {
	if len(pages) == 0 {
		return fmt.Errorf("pages: nothing to show")
	}
	owner := i.Member.User.ID
	embed, err := pages[0](Info{Cur: 1, Max: len(pages)})
	if err != nil {
		return err
	}

	man.mu.Lock()
	b := &book{pages: pages, epoch: man.nextEpoch(owner)}
	man.books[owner] = b
	man.mu.Unlock()

	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:          []*discordgo.MessageEmbed{embed},
			Components:      buttons(owner, b.epoch, len(pages)),
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	})
}

// the book a click may act on: same run, its owner clicking, and their newest
func (man *Manager) live(id clickID, clicker string) (*book, bool) {
	if id.run != run.ID || id.owner != clicker {
		return nil, false
	}
	man.mu.Lock()
	defer man.mu.Unlock()
	b, ok := man.books[id.owner]
	if !ok || b.epoch != id.epoch {
		return nil, false
	}
	return b, true
}

// the prefix handler. the click is already acknowledged (deferred update), so
// ignoring it is just returning. anything with our prefix is ours to swallow
func (man *Manager) click(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error) {
	if i.Type != discordgo.InteractionMessageComponent || i.Member == nil {
		return true, nil
	}
	id, ok := parseID(i.MessageComponentData().CustomID)
	if !ok {
		return true, nil
	}
	clicker := i.Member.User.ID
	b, ok := man.live(id, clicker)
	if !ok {
		// the owner flipping a book from before a restart or one they've
		// since replaced: drop its buttons so it reads as finished
		if id.owner == clicker {
			_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Components: &[]discordgo.MessageComponent{}})
			return true, err
		}
		return true, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	max := len(b.pages)
	cur := turn(b.cur, max, id.dir)
	embed, err := b.pages[cur](Info{Cur: cur + 1, Max: max})
	if err != nil {
		return true, err
	}
	embeds := []*discordgo.MessageEmbed{embed}
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds}); err != nil {
		return true, err
	}
	b.cur = cur
	return true, nil
}
