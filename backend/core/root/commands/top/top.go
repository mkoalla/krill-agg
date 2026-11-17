package top

import (
	"fmt"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/confirm"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/pages"
	"krill-agg/root/types"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type CommandInfo = types.CommandInfo
type Row = leaderboard.Row

const PER_PAGE = 10

const (
	ALL = "all"
)

// how each board is titled and its value column shown
type boardInfo struct {
	title  string
	column string
	value  func(r Row) string
}

func whole(r Row) string    { return strconv.Itoa(int(r.Value)) }
func oneDecim(r Row) string { return strconv.FormatFloat(r.Value, 'f', 1, 64) }

var BOARD_INFO = map[string]boardInfo{
	leaderboard.BEST:    {title: "Best Scores", column: "Score", value: whole},
	leaderboard.WORST:   {title: "Worst Scores", column: "Score", value: whole},
	leaderboard.AVERAGE: {title: "Player Averages", column: "Average", value: oneDecim},
	leaderboard.PLAYED:  {title: "Games Played", column: "Played", value: whole},
}

const UPDATES = "-# Leaderboards update at the start of every hour (next <t:%d:R>)"

func disclaimer(now time.Time) *discordgo.MessageEmbedField {
	return &discordgo.MessageEmbedField{Name: "\u200b", Value: fmt.Sprintf(UPDATES, leaderboard.NextUpdate(now).Unix())}
}

// a value cell: the value padded to the board's widest, plus "#day emojis" for
// a single game. code spans keep it all the same width line to line
func valueCell(info boardInfo, r Row, width int) string {
	cell := fmt.Sprintf("`%*s`", width, info.value(r))
	if r.Emojis != "" {
		cell += fmt.Sprintf(" `%-4s %s`", "#"+strconv.Itoa(r.Day), r.Emojis) // days left-aligned, padded to 3 digits
	}
	return cell
}

// the board's widest rank and value, so every page of it lines up the same
func widths(info boardInfo, rows []Row) (rank, value int) {
	rank = len("#" + strconv.Itoa(len(rows)))
	for _, r := range rows {
		value = max(value, len(info.value(r)))
	}
	return rank, value
}

// two side-by-side columns: "`#1 ` <@uid>" and the value. mentions inside an
// embed show the profile without pinging
func columns(info boardInfo, rows []Row, all []Row) []*discordgo.MessageEmbedField {
	rankW, valueW := widths(info, all)
	players := make([]string, len(rows))
	values := make([]string, len(rows))
	for j, r := range rows {
		players[j] = fmt.Sprintf("`%-*s` <@%s>", rankW, "#"+strconv.Itoa(r.Rank), r.UserID)
		values[j] = valueCell(info, r, valueW)
	}
	return []*discordgo.MessageEmbedField{
		{Name: "Player", Value: strings.Join(players, "\n"), Inline: true},
		{Name: info.column, Value: strings.Join(values, "\n"), Inline: true},
	}
}

// rows [p*PER_PAGE, +PER_PAGE), clamped to what the board has now
func pageOf(rows []Row, p int) []Row {
	start := min(p*PER_PAGE, len(rows))
	return rows[start:min(start+PER_PAGE, len(rows))]
}

func pageCount(players int) int {
	return max(1, (players+PER_PAGE-1)/PER_PAGE)
}

// cached first; the api only when the state is missing it
func guildName(s *discordgo.Session, guild string) string {
	if g, err := s.State.Guild(guild); err == nil && g.Name != "" {
		return g.Name
	}
	if g, err := s.Guild(guild); err == nil {
		return g.Name
	}
	return "this server"
}

// guild avatar if they have one, else their account's
func avatarURL(s *discordgo.Session, guild, user string) string {
	m, err := s.State.Member(guild, user)
	if err != nil {
		if m, err = s.GuildMember(guild, user); err != nil {
			if u, err := s.User(user); err == nil {
				return u.AvatarURL("128")
			}
			return ""
		}
	}
	return m.AvatarURL("128")
}

// one board's pages. nothing is built until a page is shown; the icon (the
// board's #1) is looked up once and shared by its pages
// "🏆 Best Scores (1 Week) in <guild>"
func boardTitle(what, period, guildName string) string {
	return fmt.Sprintf("🏆 %s (%s) in %s", what, leaderboard.PERIOD_LABELS[period], guildName)
}

func boardPages(s *discordgo.Session, bcp *types.BotCommandPackage, guild, name, board, period string, players int) []pages.Page {
	info := BOARD_INFO[board]
	title := boardTitle(info.title, period, name)
	var iconOnce sync.Once
	var icon string
	book := make([]pages.Page, pageCount(players))
	for p := range book {
		book[p] = func(at pages.Info) (*discordgo.MessageEmbed, error) {
			rows, _, err := bcp.Leaderboard.Board(guild, board, period)
			if err != nil {
				return nil, err
			}
			if len(rows) > 0 {
				iconOnce.Do(func() { icon = avatarURL(s, guild, rows[0].UserID) })
			}
			page := pageOf(rows, p)
			embed := pages.Embed(title, icon, "", at)
			if len(page) == 0 {
				embed.Description = "No players yet!"
			} else {
				embed.Fields = columns(info, page, rows)
			}
			embed.Fields = append(embed.Fields, disclaimer(time.Now()))
			return embed, nil
		}
	}
	return book
}

// what /topu says when there's nothing to rank, about the caller or someone else
func notRanked(self bool, user, you, them string) string {
	if self {
		return you
	}
	return fmt.Sprintf(them, user)
}

// the name /topu titles someone by: their server nickname, else their own
func displayName(s *discordgo.Session, guild, user string) string {
	if m, err := s.State.Member(guild, user); err == nil && m.User != nil {
		return m.DisplayName()
	}
	if m, err := s.GuildMember(guild, user); err == nil && m.User != nil {
		return m.DisplayName()
	}
	if u, err := s.User(user); err == nil {
		return u.DisplayName()
	}
	return "Someone"
}

// the caller's rank on every board
// one player's rank on every board (/topu); self says "you"
func userPage(s *discordgo.Session, bcp *types.BotCommandPackage, guild, name, user, period string, self bool) pages.Page {
	return func(at pages.Info) (*discordgo.MessageEmbed, error) {
		who := "Your Ranks"
		if !self {
			who = displayName(s, guild, user) + "'s Ranks"
		}
		title := boardTitle(who, period, name)
		if banned, err := bcp.Leaderboard.IsBlacklisted(guild, user); err != nil {
			return nil, err
		} else if banned {
			body := notRanked(self, user, "You're left off this server's leaderboards.", "<@%s> is left off this server's leaderboards.")
			return pages.Embed(title, avatarURL(s, guild, user), body, at), nil
		}
		var lines []string
		for _, board := range leaderboard.BOARDS {
			rows, _, err := bcp.Leaderboard.Board(guild, board, period)
			if err != nil {
				return nil, err
			}
			if line, ok := selfLine(BOARD_INFO[board], rows, user); ok {
				lines = append(lines, line)
			}
		}
		body := notRanked(self, user, "You haven't played yet!", "<@%s> hasn't played yet!")
		if len(lines) > 0 {
			body = strings.Join(lines, "\n")
		}
		embed := pages.Embed(title, avatarURL(s, guild, user), body, at)
		embed.Fields = append(embed.Fields, disclaimer(time.Now()))
		return embed, nil
	}
}

// "**Best Scores** — `#3` of 12 · `415` `#124 🦑…`"
func selfLine(info boardInfo, rows []Row, user string) (string, bool) {
	for _, r := range rows {
		if r.UserID == user {
			return fmt.Sprintf("**%s** — `#%d` of %d · %s", info.title, r.Rank, len(rows), valueCell(info, r, 0)), true
		}
	}
	return "", false
}

// the book for a /top choice: one board, every board back to back (each from
// a fresh page), or the caller's ranks
func book(s *discordgo.Session, bcp *types.BotCommandPackage, i *discordgo.InteractionCreate, choice, period string) ([]pages.Page, error) {
	g := i.GuildID
	name := guildName(s, g)
	players, err := bcp.Leaderboard.Players(g, period)
	if err != nil {
		return nil, err
	}
	boards := []string{choice}
	if choice == ALL {
		boards = leaderboard.BOARDS
	}
	var all []pages.Page
	for _, board := range boards {
		all = append(all, boardPages(s, bcp, g, name, board, period, players)...)
	}
	return all, nil
}

// a string or user option's value (a user's is their id), or fallback when
// it wasn't given. read raw: discordgo's StringValue panics on user options
func option(i *discordgo.InteractionCreate, name, fallback string) string {
	for _, opt := range i.ApplicationCommandData().Options {
		if v, ok := opt.Value.(string); ok && opt.Name == name {
			return v
		}
	}
	return fallback
}

func open(s *discordgo.Session, bcp *types.BotCommandPackage, i *discordgo.InteractionCreate, choice, period string) error {
	b, err := book(s, bcp, i, choice, period)
	if err != nil {
		return err
	}
	// /mg-refetchtop's book flips like /top's: whoever can run /top
	return bcp.Pages.Open(s, i, "top", b)
}

// map is already reference-like but just for continuity
func Register(bcp *types.BotCommandPackage) {
	registerBlacklist(bcp)
	choices := []*discordgo.ApplicationCommandOptionChoice{{Name: "all", Value: ALL}}
	for _, board := range leaderboard.BOARDS {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: board, Value: board})
	}
	periods := make([]*discordgo.ApplicationCommandOptionChoice, len(leaderboard.PERIODS))
	for j, p := range leaderboard.PERIODS {
		periods[j] = &discordgo.ApplicationCommandOptionChoice{Name: leaderboard.PERIOD_LABELS[p], Value: p}
	}

	(*bcp.Handles)["top"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "top",
			Description: "krillion leaderboards for this server",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "which board (default: all of them)",
				Choices:     choices,
			}, {
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "since",
				Description: "how far back to count (default: all-time)",
				Choices:     periods,
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			return open(s, bcp, i, option(i, "type", ALL), option(i, "since", leaderboard.ALLTIME))
		},
	}
	(*bcp.Handles)["topu"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "topu",
			Description: "someone's rank on every leaderboard in this server",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "whose ranks (default: yours)",
			}, {
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "since",
				Description: "how far back to count (default: all-time)",
				Choices:     periods,
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			caller := i.Member.User.ID
			user := option(i, "user", caller)
			period := option(i, "since", leaderboard.ALLTIME)
			name := guildName(s, i.GuildID)
			// one page, so it's built once (no buttons to flip it) and kept to
			// sit above the trend card
			var ranks *discordgo.MessageEmbed
			page := userPage(s, bcp, i.GuildID, name, user, period, user == caller)
			kept := func(at pages.Info) (*discordgo.MessageEmbed, error) {
				embed, err := page(at)
				ranks = embed
				return embed, err
			}
			if err := bcp.Pages.Open(s, i, "topu", []pages.Page{kept}); err != nil {
				return err
			}
			return attachTrend(s, bcp, i, ranks, name, user, period)
		},
	}
	(*bcp.Handles)["mg-cleartopu"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "mg-cleartopu",
			Description: "delete cached /topu cards so they're drawn fresh",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "whose cards (default: everyone's in this server)",
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			if user := option(i, "user", ""); user != "" {
				text, err := clearTopu(bcp, i.GuildID, []string{user}, fmt.Sprintf("<@%s>", user))
				if err != nil {
					return err
				}
				return confirm.Reply(s, i, text)
			}
			// everyone's: ask first
			return confirm.Open(s, i, confirm.Prompt{
				Flow: CLEAR_ALL,
				Question: func(int) string {
					return "Are you sure you want to clear every cached /topu card in this server?"
				},
				Cancelled: "Cancelled, nothing was cleared.",
				TimedOut:  "Time's up, nothing was cleared.",
				Confirm: func() (string, error) {
					text, err := clearTopu(bcp, i.GuildID, nil, "this server")
					if err != nil {
						return "Couldn't clear them, try again later.", err
					}
					return text, nil
				},
			})
		},
	}
	(*bcp.Handles)["mg-refetchtop"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "mg-refetchtop",
			Description: "rebuild this server's leaderboards now, then show them",
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			if err := bcp.Leaderboard.Refetch(i.GuildID); err != nil {
				return err
			}
			return open(s, bcp, i, ALL, leaderboard.ALLTIME)
		},
	}
}

// server-wide /mg-cleartopu: one Confirm click within the default 30s
var CLEAR_ALL = confirm.Flow{Steps: 1, Timeout: confirm.DEFAULT_TIMEOUT}

// clears users' cards (everyone's if none) and says how it went; whose is
// how the reply names them. replies are embeds, so mentions never ping
func clearTopu(bcp *types.BotCommandPackage, guild string, users []string, whose string) (string, error) {
	cards, busy, err := clearTrends(bcp, guild, users)
	if err != nil {
		return "", err
	}
	text := fmt.Sprintf("Cleared %d cached /topu card%s for %s.", cards, plural(cards), whose)
	for _, b := range busy {
		text += fmt.Sprintf("\n-# Skipped <@%s>: a card is being drawn right now.", b)
	}
	return text, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
