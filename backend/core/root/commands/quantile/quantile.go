// Package quantile answers what score beats p% of games (the p-th
// percentile), in this server and across every krillion player, all-time.
//
// The server's answer assumes its scores are normal: its mean plus z
// standard deviations, from running totals the scores manager keeps (no full
// read per ask). Everyone's comes from krillion's saved histograms, added up:
// the same in-bucket interpolation the percentiles use, run backwards. Both
// carry a confidence interval for the estimate, again assuming normal scores.
package quantile

import (
	"fmt"
	"krill-agg/root/info/krillionapi"
	"krill-agg/root/info/theme"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"math"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo

// the confidence interval when none is asked for, in percent
const DEFAULT_CI = 90

// Φ⁻¹: the z a share p (0-1) of a standard normal falls under
func probit(p float64) float64 {
	return math.Sqrt2 * math.Erfinv(2*p-1)
}

// a quantile and the half-width of its confidence interval
type estimate struct {
	score float64
	err   float64
}

// the score a share q (0-1) of normal scores with this mean and standard
// deviation fall under, and how far off it may be from n games: the estimate
// mean + z·sd varies by sd²/n through the mean and z²·sd²/2(n-1) through sd.
// ci is the interval's coverage (0-1)
func normalError(sd float64, n int64, q, ci float64) float64 {
	z := probit(q)
	se := sd * math.Sqrt(1/float64(n)+z*z/(2*float64(n-1)))
	return probit(0.5+ci/2) * se
}

// the highest a day can score: the top of krillion's last bucket
const MAX_SCORE = 20 * krillionapi.BUCKET_WIDTH

// a normal runs past the board; scores don't
func clamp(score float64) float64 {
	return min(max(score, 0), MAX_SCORE)
}

// the server's top-p score from its running totals; ok=false under two games
func guildEstimate(m scores.Moments, q, ci float64) (estimate, bool) {
	if m.N < 2 {
		return estimate{}, false
	}
	n := float64(m.N)
	mean := float64(m.Sum) / n
	variance := max(float64(m.SumSq)-float64(m.Sum)*float64(m.Sum)/n, 0) / (n - 1)
	sd := math.Sqrt(variance)
	return estimate{score: clamp(mean + probit(q)*sd), err: normalError(sd, m.N, q, ci)}, true
}

// everyone's top-p score from the summed histograms; ok=false without one.
// the spread for the interval comes from the buckets' midpoints
func globalEstimate(all krillionapi.Stats, q, ci float64) (estimate, bool) {
	score, ok := krillionapi.Quantile(all.Dist, q)
	if !ok || all.Sample < 2 {
		return estimate{}, false
	}
	total, sum := 0.0, 0.0
	for b, count := range all.Dist {
		total += count
		sum += count * (float64(b) + 0.5) * krillionapi.BUCKET_WIDTH
	}
	mean, sq := sum/total, 0.0
	for b, count := range all.Dist {
		d := (float64(b)+0.5)*krillionapi.BUCKET_WIDTH - mean
		sq += count * d * d
	}
	sd := math.Sqrt(sq / total)
	return estimate{score: clamp(score), err: normalError(sd, int64(all.Sample), q, ci)}, true
}

// "412 ± 9"
func (e estimate) String() string {
	return fmt.Sprintf("%.0f ± %.0f", e.score, e.err)
}

// a percent as typed: no trailing zeros
func percent(p float64) string {
	return strconv.FormatFloat(p, 'f', -1, 64)
}

// the embed: one line per side, then what the ± means
func embed(guildName string, guild, global *estimate, beat, ci float64) *discordgo.MessageEmbed {
	line := func(name string, e *estimate, none string) string {
		if e == nil {
			return fmt.Sprintf("**%s**: *%s*", name, none)
		}
		return fmt.Sprintf("**%s**: %s", name, e)
	}
	lines := []string{
		line(guildName, guild, "not enough games yet"),
		line("Global", global, "no krillion histograms saved yet"),
		fmt.Sprintf("-# %s%% confidence interval", percent(ci)),
	}
	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Krillion Scores to Beat %s%%", percent(beat)),
		Description: strings.Join(lines, "\n"),
		Color:       theme.ACCENT,
	}
}

// a number option's value, or fallback when it wasn't given
func number(i *discordgo.InteractionCreate, name string, fallback float64) float64 {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == name && opt.Type == discordgo.ApplicationCommandOptionNumber {
			return opt.FloatValue()
		}
	}
	return fallback
}

// cached first; the api only when the state is missing it
func guildName(s *discordgo.Session, guild string) string {
	if g, err := s.State.Guild(guild); err == nil && g.Name != "" {
		return g.Name
	}
	if g, err := s.Guild(guild); err == nil {
		return g.Name
	}
	return "This server"
}

func reply(s *discordgo.Session, i *discordgo.InteractionCreate, data *discordgo.InteractionResponseData) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	})
}

var zero = 0.0

func Register(bcp *types.BotCommandPackage) {
	(*bcp.Handles)["quantile"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "quantile",
			Description: "What score beats p% of games (0-100)",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionNumber,
				Name:        "p",
				Description: "the share of games to beat, e.g. 90 or 99.5",
				Required:    true,
				MinValue:    &zero,
				MaxValue:    100,
			}, {
				Type:        discordgo.ApplicationCommandOptionNumber,
				Name:        "ci",
				Description: fmt.Sprintf("the confidence interval, in percent (default: %d)", DEFAULT_CI),
				MinValue:    &zero,
				MaxValue:    100,
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			beat, ci := number(i, "p", 0), number(i, "ci", DEFAULT_CI)
			// discord allows the ends; the normal has no 0th or 100th percentile
			if beat <= 0 || beat >= 100 || ci <= 0 || ci >= 100 {
				return reply(s, i, &discordgo.InteractionResponseData{
					Content: "p and ci must both be between 0 and 100, not either end.",
					Flags:   discordgo.MessageFlagsEphemeral,
				})
			}
			q := beat / 100 // the share of games under the score
			m, err := bcp.Scores.Moments(i.GuildID)
			if err != nil {
				return err
			}
			var guild, global *estimate
			if e, ok := guildEstimate(m, q, ci/100); ok {
				guild = &e
			}
			if all, ok := bcp.Distributions.AllTime(); ok {
				if e, ok := globalEstimate(all, q, ci/100); ok {
					global = &e
				}
			}
			return reply(s, i, &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed(guildName(s, i.GuildID), guild, global, beat, ci)},
			})
		},
	}
}
