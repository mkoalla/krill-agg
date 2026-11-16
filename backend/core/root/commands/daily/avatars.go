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

// writes <dir>/<guild>/images/<i>.png for each user, in order (dir is data/daily),
// and the guild's icon as ICON_FILE. the folder is wiped first so a failed
// download leaves a gap (grey disc), never a stale face. only failing to set up
// the folder is an error; icon is ICON_FILE if it was saved, else empty
func SaveAvatars(s *discordgo.Session, dailyDir string, g *discordgo.Guild, userIDs []string) (icon string, missing int, err error) {
	guild := g.ID
	dir := filepath.Join(dailyDir, guild, "images")
	if err := os.RemoveAll(dir); err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, err
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
	return icon, missing, nil
}
