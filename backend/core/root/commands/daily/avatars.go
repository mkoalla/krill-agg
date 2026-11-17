package daily

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	AVATAR_SIZE = "128"
	MAX_AVATAR  = 4 << 20
)

var cdn = &http.Client{Timeout: 10 * time.Second}

// always a png (discordgo's AvatarURL hands back .gif for animated ones):
// guild avatar, else account avatar, else discord's default
func pngAvatarURL(guild string, m *discordgo.Member) string {
	switch {
	case m.Avatar != "":
		return discordgo.EndpointGuildMemberAvatar(guild, m.User.ID, m.Avatar) + "?size=" + AVATAR_SIZE
	case m.User.Avatar != "":
		return discordgo.EndpointUserAvatar(m.User.ID, m.User.Avatar) + "?size=" + AVATAR_SIZE
	default:
		return discordgo.EndpointDefaultUserAvatar(m.User.DefaultAvatarIndex())
	}
}

// the guild's icon as a png (animated ones are still served as png); empty if it has none
func pngIconURL(g *discordgo.Guild) string {
	if g == nil || g.Icon == "" {
		return ""
	}
	return discordgo.EndpointGuildIcon(g.ID, g.Icon) + "?size=" + AVATAR_SIZE
}

func download(url string, to string) error {
	resp, err := cdn.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cdn %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MAX_AVATAR))
	if err != nil {
		return err
	}
	return os.WriteFile(to, raw, 0o644)
}

const ICON_FILE = "guild.png"

// makes a fresh images folder in dayDir (data/dailygen/<guild>/<day>) and
// writes <i>.png for each user, in order, plus the guild's icon as ICON_FILE.
// the folder is new each time, so concurrent renders never share it and a
// failed download is a gap (grey disc), never someone else's face. the caller
// removes it once the card is drawn. only failing to make it is an error;
// icon is ICON_FILE if it was saved, else empty
func SaveAvatars(s *discordgo.Session, dayDir string, g *discordgo.Guild, userIDs []string) (dir string, icon string, missing int, err error) {
	guild := g.ID
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		return "", "", 0, err
	}
	if dir, err = os.MkdirTemp(dayDir, "images-"); err != nil {
		return "", "", 0, err
	}
	// mkdtemp is owner-only; the renderer (another container) reads it
	if err := os.Chmod(dir, 0o755); err != nil {
		os.RemoveAll(dir) //nolint
		return "", "", 0, err
	}
	if url := pngIconURL(g); url != "" && download(url, filepath.Join(dir, ICON_FILE)) == nil {
		icon = ICON_FILE
	}
	for i, uid := range userIDs {
		member, err := s.GuildMember(guild, uid)
		if err != nil {
			// left the server: fall back to their account
			user, errUser := s.User(uid)
			if errUser != nil {
				missing++
				continue
			}
			member = &discordgo.Member{User: user}
		}
		if download(pngAvatarURL(guild, member), filepath.Join(dir, fmt.Sprintf("%d.png", i))) != nil {
			missing++
		}
	}
	return dir, icon, missing, nil
}
