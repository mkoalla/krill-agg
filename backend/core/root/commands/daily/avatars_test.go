package daily

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestPngAvatarURL(t *testing.T) {
	user := func(avatar string) *discordgo.User {
		return &discordgo.User{ID: "80351110224678912", Avatar: avatar, Discriminator: "0"}
	}
	cases := map[string]struct {
		member *discordgo.Member
		want   string
	}{
		"guild avatar wins": {&discordgo.Member{Avatar: "a_g1", User: user("u1")},
			"https://cdn.discordapp.com/guilds/7/users/80351110224678912/avatars/a_g1.png?size=128"},
		"account avatar, animated still png": {&discordgo.Member{User: user("a_u1")},
			"https://cdn.discordapp.com/avatars/80351110224678912/a_u1.png?size=128"},
		"default": {&discordgo.Member{User: user("")},
			"https://cdn.discordapp.com/embed/avatars/" + string(rune('0'+(80351110224678912>>22)%6)) + ".png"},
	}
	for name, c := range cases {
		if got := pngAvatarURL("7", c.member); got != c.want {
			t.Errorf("%s: got %s, want %s", name, got, c.want)
		}
	}
}

func TestPngIconURL(t *testing.T) {
	if got := pngIconURL(&discordgo.Guild{ID: "7"}); got != "" {
		t.Errorf("no icon: got %q", got)
	}
	if got, want := pngIconURL(&discordgo.Guild{ID: "7", Icon: "a_i1"}), "https://cdn.discordapp.com/icons/7/a_i1.png?size=128"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
