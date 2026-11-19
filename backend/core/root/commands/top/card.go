package top

import (
	"errors"
	"fmt"
	"krill-agg/root/commands/daily"
	"os"
	"path/filepath"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// draws the card at path (with draw) unless it's already on disk, under
// guild/who's claim (see claimDraw). onDraw goes first and returns a link to
// the reply that will carry it; if who's card is already being drawn, nothing
// is and the error is a *drawingError. once drawn, the other cards (and
// links) matching stale, a glob, go
func cachedCard(path, guild, who, stale string, onDraw func() (string, error), draw func() error) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	publish, done, err := claimDraw(guild, who)
	if err != nil {
		return err
	}
	defer done()
	link, err := onDraw()
	if err != nil {
		return err
	}
	publish(link)
	if err := draw(); err != nil {
		return err
	}
	keep := strings.TrimSuffix(path, ".webp")
	old, _ := filepath.Glob(stale)
	for _, f := range old {
		if strings.TrimSuffix(f, filepath.Ext(f)) != keep {
			os.Remove(f) //nolint // a leftover is only disk space
		}
	}
	return nil
}

// adds the card at path under head, an embed already sent as the reply: a
// placeholder while it draws (see cachedCard), then the card, or why there
// isn't one. a card already posted is linked again if discord still has it,
// else one already drawn is uploaded again, with no placeholder either way.
// what names the card in errors
func attachCard(s *discordgo.Session, i *discordgo.InteractionCreate, head *discordgo.MessageEmbed, path, who, stale, what string, draw func() error) error {
	guild := i.GuildID
	noPings := &discordgo.MessageAllowedMentions{}
	send := func(image *discordgo.MessageEmbed, files []*discordgo.File) (*discordgo.Message, error) {
		embeds := []*discordgo.MessageEmbed{head, image}
		return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds, Files: files, AllowedMentions: noPings})
	}
	fail := func(reason error) error {
		_, err := send(imageEmbed("-# couldn't produce image: "+reason.Error(), ""), nil)
		return errors.Join(fmt.Errorf("%s: %v", what, reason), err)
	}

	if url, at, ok := daily.ReadLink(trendLink(path)); ok {
		_, err := send(imageEmbed(fmt.Sprintf("-# Image reused from <t:%d:R>", at.Unix()), url), nil)
		return err
	}
	placed := false // the placeholder went out, so a failure has a message to edit
	err := cachedCard(path, guild, who, stale, func() (string, error) {
		msg, err := send(imageEmbed("-# generating image...", ""), nil)
		if err != nil {
			return "", err
		}
		placed = true
		return daily.JumpLink(guild, msg), nil
	}, draw)
	var drawing *drawingError
	if errors.As(err, &drawing) {
		// like /forcedaily: point at the draw underway instead of starting another
		note := "-# Already generating"
		if drawing.link != "" {
			note += ": " + drawing.link
		}
		_, err := send(imageEmbed(note, ""), nil)
		return err
	}
	if err != nil {
		if !placed {
			return err
		}
		return fail(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(err)
	}
	defer f.Close() //nolint
	base := filepath.Base(path)
	final, err := send(imageEmbed("", "attachment://"+base), []*discordgo.File{{Name: base, ContentType: "image/webp", Reader: f}})
	if err != nil {
		return fail(err)
	}
	// head is embed 0, the card 1
	var url string
	if len(final.Embeds) > 1 && final.Embeds[1].Image != nil && strings.HasPrefix(final.Embeds[1].Image.URL, "http") {
		url = final.Embeds[1].Image.URL
	} else if len(final.Attachments) > 0 {
		url = final.Attachments[0].URL
	}
	if url == "" {
		return fmt.Errorf("%s: posted, but discord returned no url to reuse", what)
	}
	if err := daily.WriteLink(trendLink(path), url); err != nil {
		return fmt.Errorf("%s: saving its link: %v", what, err)
	}
	return nil
}
