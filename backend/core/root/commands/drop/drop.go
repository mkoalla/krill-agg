// Package drop is /drop and /mg-drop: deleting your own results, or (for
// managers) someone else's, one day or all of them, behind confirmation
// (see DROP).
package drop

import (
	"fmt"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/confirm"
	"krill-agg/root/types"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo

// two Confirm clicks, "sure" then "REALLY sure", each with its own 30s
var DROP = confirm.Flow{Steps: 2, Timeout: confirm.DEFAULT_TIMEOUT}

// whose scores a drop is about, worded for the reply: "you" for your own,
// a mention otherwise. replies are embeds, so mentions never ping
type whose struct {
	user string
	self bool
}

// "your" / "<@id>'s"
func (w whose) their() string {
	if w.self {
		return "your"
	}
	return fmt.Sprintf("<@%s>'s", w.user)
}

// "You don't" / "<@id> doesn't"
func (w whose) doesnt() string {
	if w.self {
		return "You don't"
	}
	return fmt.Sprintf("<@%s> doesn't", w.user)
}

// "Are you sure you want to drop ALL your scores?\n- 12 scores"
func question(what, shown string) func(step int) string {
	return func(step int) string {
		sure := "sure"
		if step > 1 {
			sure = "REALLY sure"
		}
		return fmt.Sprintf("Are you %s you want to drop %s?\n- %s", sure, what, shown)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// asks to drop target's scores in the guild (day 0: every day), or just
// says there's nothing to drop
func start(bcp *types.BotCommandPackage, s *discordgo.Session, i *discordgo.InteractionCreate, target string, day int) error {
	guild := i.GuildID
	w := whose{user: target, self: target == i.Member.User.ID}
	var what, shown string
	if day == 0 {
		n, err := bcp.Scores.Count(guild, target)
		if err != nil {
			return err
		}
		if n == 0 {
			return confirm.Reply(s, i, w.doesnt()+" have any scores to drop.")
		}
		what, shown = "ALL "+w.their()+" scores", fmt.Sprintf("%d score%s", n, plural(n))
	} else {
		score, ok, err := bcp.Scores.Get(guild, day, target)
		if err != nil {
			return err
		}
		if !ok {
			return confirm.Reply(s, i, w.doesnt()+" have a score for this day.")
		}
		what, shown = fmt.Sprintf("%s score for day %d", w.their(), day), score.Format()
	}
	return confirm.Open(s, i, confirm.Prompt{
		Flow:      DROP,
		Question:  question(what, shown),
		Cancelled: "Cancelled, nothing was dropped.",
		TimedOut:  "Time's up, nothing was dropped.",
		Confirm: func() (string, error) {
			text, err := drop(bcp, guild, w, day, shown)
			if err != nil {
				return "Couldn't drop that, try again later.", fmt.Errorf("dropping %s: %v", what, err)
			}
			// the boards would keep showing it until the hour turns otherwise
			return text, bcp.Leaderboard.Refetch(guild)
		},
	})
}

// does the drop; says what was dropped
func drop(bcp *types.BotCommandPackage, guild string, w whose, day int, shown string) (string, error) {
	if day == 0 {
		n, err := bcp.Scores.DropAll(guild, w.user)
		return fmt.Sprintf("Dropped all %d of %s score%s.", n, w.their(), plural(n)), err
	}
	ok, err := bcp.Scores.Drop(guild, w.user, day)
	if err == nil && !ok {
		return w.doesnt() + " have a score for this day anymore.", nil
	}
	return fmt.Sprintf("Dropped %s score for day %d.\n- %s", w.their(), day, shown), err
}

// the day option, resolved (krillion.Resolve); 0 when it's left out, which
// means every day. ok=false for a day before krillion #1
func dayOption(i *discordgo.InteractionCreate) (day int, ok bool) {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "day" {
			day = krillion.Resolve(int(opt.IntValue()))
			return day, day >= 1
		}
	}
	return 0, true
}

const BEFORE_DAY_ONE = "That's before Krillion #1."

func Register(bcp *types.BotCommandPackage) {
	(*bcp.Handles)["drop"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "drop",
			Description: "delete your own results in this server",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "day",
				Description: krillion.DAY_OPTION + " (default: all of them)",
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			day, ok := dayOption(i)
			if !ok {
				return confirm.Reply(s, i, BEFORE_DAY_ONE)
			}
			return start(bcp, s, i, i.Member.User.ID, day)
		},
	}
	(*bcp.Handles)["mg-drop"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "mg-drop",
			Description: "delete someone's results in this server",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "whose results",
				Required:    true,
			}, {
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "day",
				Description: krillion.DAY_OPTION + " (default: all of them)",
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			var user string
			for _, opt := range i.ApplicationCommandData().Options {
				if opt.Name == "user" {
					user, _ = opt.Value.(string) // raw: UserValue needs a session lookup
				}
			}
			day, ok := dayOption(i)
			if !ok {
				return confirm.Reply(s, i, BEFORE_DAY_ONE)
			}
			return start(bcp, s, i, user, day)
		},
	}
}
