package daily

import (
	"errors"
	"fmt"
	"krill-agg/root/ctl"
	"krill-agg/root/types"
	"net/url"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// `main ctl daily <channel>...`: posts today's report in each channel, one at
// a time. every channel is tried; the error lists the ones that failed
func CtlDaily(s *discordgo.Session, bcp *types.BotCommandPackage) ctl.Handler {
	return func(args url.Values) (string, error) {
		channels := args["channel"]
		if len(channels) == 0 {
			return "", errors.New("no channel given")
		}
		var posted []string
		var errs []error
		for _, channel := range channels {
			link, err := PostDaily(s, bcp, channel)
			if link != "" {
				posted = append(posted, "posted "+link)
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %v", channel, err))
			}
		}
		out := strings.Join(posted, "\n")
		if len(errs) > 0 {
			if out != "" {
				errs = append([]error{errors.New(out)}, errs...)
			}
			return "", errors.Join(errs...)
		}
		return out, nil
	}
}
