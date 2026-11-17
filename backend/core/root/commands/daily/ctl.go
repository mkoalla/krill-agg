package daily

import (
	"errors"
	"fmt"
	"krill-agg/root/ctl"
	"krill-agg/root/types"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// guilds posted at once by `ctl daily`. each one is mostly waiting on
// krillion, discord and the renderer, so a few overlap well
const CTL_WORKERS = 5

// swapped in tests
var postDaily = PostDaily

type ctlResult struct {
	link string
	err  error
}

// posts in every channel, at most workers at a time. results line up with channels
func postAll(channels []string, workers int, post func(channel string) (string, error)) []ctlResult {
	results := make([]ctlResult, len(channels))
	slots := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for j, channel := range channels {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			link, err := post(channel)
			results[j] = ctlResult{link, err}
		})
	}
	wg.Wait()
	return results
}

// `main ctl daily`: posts today's report in every guild's main channel (see
// /mg-mainchan), CTL_WORKERS at a time. every guild is tried. the first line of
// the output (or error) is a summary; links or failures follow it, one per line.
// logf announces the run before it starts
func CtlDaily(s *discordgo.Session, bcp *types.BotCommandPackage, logf func(msg string)) ctl.Handler {
	return func(url.Values) (string, error) {
		mains := bcp.Tracking.Mains()
		if len(mains) == 0 {
			return "no guild has a main channel; nothing posted", nil
		}
		guilds := slices.Sorted(maps.Keys(mains))
		channels := make([]string, len(guilds))
		for j, g := range guilds {
			channels[j] = mains[g]
		}
		logf(fmt.Sprintf("ctl daily: posting to %d channel(s)", len(channels)))

		results := postAll(channels, CTL_WORKERS, func(channel string) (string, error) {
			return postDaily(s, bcp, channel)
		})
		var links, failures []string
		for j, r := range results {
			if r.link != "" {
				links = append(links, "posted "+r.link)
			}
			if r.err != nil {
				failures = append(failures, fmt.Sprintf("guild %s (channel %s): %v", guilds[j], channels[j], r.err))
			}
		}
		// a post can go out and still fail (e.g. its image), so it counts as failed
		ok := len(channels) - len(failures)
		summary := fmt.Sprintf("posted to %d channel(s) (%d failed)", ok, len(failures))
		if len(failures) > 0 {
			return "", errors.New(strings.Join(append([]string{summary}, append(failures, links...)...), "\n"))
		}
		return strings.Join(append([]string{summary}, links...), "\n"), nil
	}
}
