package daily

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestImageCache(t *testing.T) {
	now := time.Now()
	remember("g", cachedImage{day: 124, at: now.Add(-10 * time.Minute), url: "https://cdn/x.png"})
	if c, ok := cached("g", 124, now); !ok || c.url != "https://cdn/x.png" {
		t.Fatalf("fresh same-day card not reused: %+v %v", c, ok)
	}
	if _, ok := cached("g", 125, now); ok {
		t.Error("reused across days")
	}
	if _, ok := cached("g", 124, now.Add(25*time.Minute)); ok {
		t.Error("reused after 30 minutes")
	}
	if _, ok := cached("other", 124, now); ok {
		t.Error("reused across guilds")
	}
}

func TestClaimOnePerGuild(t *testing.T) {
	now := time.Now()
	lead := claim("w", 124, now, false)
	if lead.lead == nil {
		t.Fatal("first claim should lead")
	}
	// any day: one render per guild
	for _, day := range []int{124, 125} {
		if f := claim("w", day, now, false); f.follow != lead.lead {
			t.Fatalf("day %d: a claim during the render should follow it", day)
		}
	}
	if f := claim("w", 124, now, true); f.follow != lead.lead {
		t.Fatal("fresh skips the cache, not the one-render rule")
	}
	if other := claim("x", 124, now, false); other.lead == nil {
		t.Fatal("other guilds aren't blocked")
	} else {
		other.done()
	}

	follower := claim("w", 124, now, false)
	lead.lead.publish("https://discord.com/channels/1/2/3")
	<-follower.follow.posted
	if follower.follow.link != "https://discord.com/channels/1/2/3" {
		t.Fatalf("follower got link %q", follower.follow.link)
	}
	remember("w", cachedImage{day: 124, at: now, url: "https://cdn/w.png"})
	lead.done()
	lead.done() // idempotent
	if c := claim("w", 124, now, false); c.cached == nil || c.cached.url != "https://cdn/w.png" {
		t.Fatal("after the render, the next claim should reuse it")
	}
	if c := claim("w", 124, now, true); c.lead == nil {
		t.Fatal("fresh should render despite the cache")
	} else {
		c.done()
	}
}

// a leader that never replies releases its followers with no link
func TestClaimLeaderFailsBeforeReply(t *testing.T) {
	lead := claim("f", 124, time.Now(), false)
	follower := claim("f", 124, time.Now(), false)
	lead.done()
	<-follower.follow.posted
	if follower.follow.link != "" {
		t.Fatal("expected no link")
	}
	if again := claim("f", 124, time.Now(), false); again.lead == nil {
		t.Fatal("the retry should lead")
	} else {
		again.done()
	}
}

// many simultaneous calls: exactly one leads
func TestClaimRace(t *testing.T) {
	var wg sync.WaitGroup
	var leads atomic.Int32
	tickets := make(chan ticket, 64)
	for range 64 {
		wg.Go(func() {
			tk := claim("r", 124, time.Now(), false)
			if tk.lead != nil {
				leads.Add(1)
			}
			tickets <- tk
		})
	}
	wg.Wait()
	close(tickets)
	if leads.Load() != 1 {
		t.Fatalf("%d leaders", leads.Load())
	}
	for tk := range tickets {
		tk.done()
	}
}

func TestPostedURL(t *testing.T) {
	msg := &discordgo.Message{
		Embeds:      []*discordgo.MessageEmbed{{Description: "board"}, {Image: &discordgo.MessageEmbedImage{URL: "https://cdn/e.png"}}},
		Attachments: []*discordgo.MessageAttachment{{URL: "https://cdn/a.png"}},
	}
	if got := postedURL(msg); got != "https://cdn/e.png" {
		t.Errorf("got %s, want the embed's url", got)
	}
	msg.Embeds = nil
	if got := postedURL(msg); got != "https://cdn/a.png" {
		t.Errorf("got %s, want the attachment's url", got)
	}
}
