package top

import (
	"context"
	"errors"
	"fmt"
	"krill-agg/root/commands/daily"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/krillion"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/confirm"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/scores"
	"krill-agg/root/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// /compare: two players' ranks on every board side by side, over a card of
// their scores day by day (dailygen's /compare), or one player against their
// guild's daily average. like /topg, only finished days are drawn, and the
// cards are cached a day at a time

// the right side when it's the guild, in card names and claims. player ids
// are all digits, so it never meets one
const GUILD_SIDE = SERVER

// a row's winner, before their cell
const WINNER = "💪"

// who /compare puts side by side, from its options: the left id is the lower
// as a string (so a pair always caches under one name), the right one is
// GUILD_SIDE against the guild. none given is the caller against the guild,
// one is the caller against them. ok=false for someone against themselves
func pairOf(caller, user1, user2 string) (left, right string, ok bool) {
	switch {
	case user1 == "" && user2 == "":
		return caller, GUILD_SIDE, true
	case user1 == "":
		user1 = caller
	case user2 == "":
		user2 = caller
	}
	if user1 == user2 {
		return "", "", false
	}
	if user1 > user2 {
		user1, user2 = user2, user1
	}
	return user1, user2, true
}

// "`#3. 415`", or "`—`" for someone off the board
func versusCell(info boardInfo, r *Row) string {
	if r == nil {
		return "`—`"
	}
	return fmt.Sprintf("`#%d. %s`", r.Rank, info.value(*r))
}

func findRow(rows []Row, user string) *Row {
	for j := range rows {
		if rows[j].UserID == user {
			return &rows[j]
		}
	}
	return nil
}

// which side takes a row: the better (lower) rank, or the only one ranked.
// none on a tie or with neither ranked
func rowWinner(l, r *Row) (left, right bool) {
	switch {
	case l == nil && r == nil:
		return false, false
	case r == nil:
		return true, false
	case l == nil:
		return false, true
	}
	return l.Rank < r.Rank, r.Rank < l.Rank
}

// one board's rows, by name (leaderboard.BOARDS)
type boardRows = map[string][]Row

// the ranks side by side: board names, then a column per side headed by the
// player's mention (embed field names can't mention). against the guild, its
// column is headed by guildName with "-" in every row, and the player takes
// every row they're ranked on
func versusFields(boards boardRows, left, right, guildName string) []*discordgo.MessageEmbedField {
	guildMode := right == GUILD_SIDE
	const blank = "\u200b" // a zero-width space: discord drops empty names and lines
	names := []string{blank}
	lefts := []string{fmt.Sprintf("<@%s>", left)}
	rights := []string{fmt.Sprintf("<@%s>", right)}
	if guildMode {
		rights[0] = guildName
	}
	for _, board := range leaderboard.BOARDS {
		info := BOARD_INFO[board]
		names = append(names, "**"+info.title+"**")
		l := findRow(boards[board], left)
		var r *Row
		if !guildMode {
			r = findRow(boards[board], right)
		}
		lw, rw := rowWinner(l, r)
		cell := func(row *Row, won bool) string {
			if won {
				return WINNER + " " + versusCell(info, row)
			}
			return versusCell(info, row)
		}
		lefts = append(lefts, cell(l, lw))
		if guildMode {
			rights = append(rights, "`-`")
		} else {
			rights = append(rights, cell(r, rw))
		}
	}
	return []*discordgo.MessageEmbedField{
		{Name: blank, Value: strings.Join(names, "\n"), Inline: true},
		{Name: blank, Value: strings.Join(lefts, "\n"), Inline: true},
		{Name: blank, Value: strings.Join(rights, "\n"), Inline: true},
	}
}

// under the leaderboards' own note: the card is a day behind
const GRAPH_NOTE = "-# The graph updates daily, and may be slightly out-of-date."

// "alice vs bob", or "alice vs Krill Club". titles can't mention, so names
func versusTitle(leftName, rightName string) string {
	return leftName + " vs " + rightName
}

// the period under the title; all-time, the default, goes without saying
func periodNote(period string) string {
	if period == leaderboard.ALLTIME {
		return ""
	}
	return "-# " + leaderboard.PERIOD_LABELS[period]
}

// one player's finished games from minDay through last, as a side (no name)
func playerSide(entries []scores.Entry, user string, minDay, last int) dailygen.CompareSide {
	side := dailygen.CompareSide{}
	sum := 0
	for _, g := range history(entries, user) {
		if g.Day < minDay || g.Day > last {
			continue
		}
		side.Points = append(side.Points, dailygen.TrendAverage{Day: g.Day, Value: float64(g.Score)})
		sum += g.Score
	}
	if side.Dives = len(side.Points); side.Dives > 0 {
		avg := float64(sum) / float64(side.Dives)
		side.Average = &avg
	}
	return side
}

// the guild's daily averages from minDay through last, leaving out banned
// players, as a side (no name)
func guildSide(entries []scores.Entry, banned map[string]bool, minDay, last int) dailygen.CompareSide {
	type tally struct{ sum, n int }
	days := map[int]*tally{}
	side := dailygen.CompareSide{Guild: true}
	sum := 0
	for _, e := range entries {
		d := e.Score.Day
		if d < minDay || d > last || banned[e.UserID] {
			continue
		}
		if days[d] == nil {
			days[d] = &tally{}
		}
		days[d].sum += e.Score.Score
		days[d].n++
		sum += e.Score.Score
		side.Dives++
	}
	for _, d := range slices.Sorted(maps.Keys(days)) {
		side.Points = append(side.Points, dailygen.TrendAverage{Day: d, Value: float64(days[d].sum) / float64(days[d].n)})
	}
	if side.Dives > 0 {
		avg := float64(sum) / float64(side.Dives)
		side.Average = &avg
	}
	return side
}

// both sides over days first..last: first is the earlier side's first game.
// ok=false when neither has one
func compareSpan(l, r *dailygen.CompareSide, last int) (first int, window string, ok bool) {
	first = last + 1
	for _, s := range []*dailygen.CompareSide{l, r} {
		if len(s.Points) > 0 {
			first = min(first, s.Points[0].Day)
		}
	}
	if first > last {
		return 0, "", false
	}
	days, label := maWindow(last - first + 1)
	l.MA, r.MA = seriesAverage(l.Points, days), seriesAverage(r.Points, days)
	return first, label, true
}

// a guild's /compare cards
func compareDir(bcp *types.BotCommandPackage, guild string) string {
	return filepath.Join(bcp.DataDir, dailygen.DIR, guild, "compare")
}

// "compare_<left>_<right>_<period>": a pair's cards for a period, each day's
// named by its last day
func comparePrefix(left, right, period string) string {
	return fmt.Sprintf("compare_%s_%s_%s", left, right, period)
}

// the card for a pair over days through last: a new one each day
func comparePath(bcp *types.BotCommandPackage, guild, left, right, period string, last int) string {
	return filepath.Join(compareDir(bcp, guild), fmt.Sprintf("%s_%d.webp", comparePrefix(left, right, period), last))
}

// draws left against right (a player, or GUILD_SIDE) as path, each line
// ending in its avatar (the guild's in its icon), players by their nicknames
func compareImage(s *discordgo.Session, path, guild, left, right string, req dailygen.CompareRequest) error {
	g, users := &discordgo.Guild{ID: guild}, []string{left}
	if right == GUILD_SIDE {
		g = guildOf(s, guild)
	} else {
		users = append(users, right)
	}
	images, icon, _, err := daily.SaveAvatars(s, filepath.Dir(path), g, users)
	if err != nil {
		return fmt.Errorf("saving avatars: %v", err)
	}
	defer os.RemoveAll(images) //nolint // only needed while drawing
	req.Images = filepath.Base(images)
	req.Left.Name, req.Left.Avatar = displayName(s, guild, left), "0.png"
	if right == GUILD_SIDE {
		req.Right.Avatar = icon // its name is the guild's, already in
	} else {
		req.Right.Name, req.Right.Avatar = displayName(s, guild, right), "1.png"
	}
	ctx, cancel := context.WithTimeout(context.Background(), TREND_TIMEOUT)
	defer cancel()
	_, err = dailygen.RenderCompare(ctx, req)
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("the renderer took longer than %v", TREND_TIMEOUT)
	}
	return err
}

// /compare: the ranks right away, then the card under them (see attachCard)
func openCompare(s *discordgo.Session, bcp *types.BotCommandPackage, i *discordgo.InteractionCreate, left, right, period string) error {
	guild := i.GuildID
	boards := boardRows{}
	for _, board := range leaderboard.BOARDS {
		rows, _, err := bcp.Leaderboard.Board(guild, board, period)
		if err != nil {
			return err
		}
		boards[board] = rows
	}
	name := guildName(s, guild)
	guildMode := right == GUILD_SIDE
	rightName := name
	if !guildMode {
		rightName = displayName(s, guild, right)
	}
	head := imageEmbed(periodNote(period), "")
	head.Title = versusTitle(displayName(s, guild, left), rightName)
	note := disclaimer(time.Now())
	note.Value += "\n" + GRAPH_NOTE
	head.Fields = append(versusFields(boards, left, right, name), note)

	today := krillion.Today()
	minDay, last := leaderboard.MinDay(period, today), today-1
	entries, err := bcp.Scores.Guild(guild, nil, minDay)
	if err != nil {
		return err
	}
	l := playerSide(entries, left, minDay, last)
	var r dailygen.CompareSide
	if right == GUILD_SIDE {
		banned, err := bcp.Leaderboard.Blacklist(guild)
		if err != nil {
			return err
		}
		set := map[string]bool{}
		for _, b := range banned {
			set[b] = true
		}
		r = guildSide(entries, set, minDay, last)
		r.Name = name
	} else {
		r = playerSide(entries, right, minDay, last)
	}
	first, window, drawable := compareSpan(&l, &r, last)

	embeds := []*discordgo.MessageEmbed{head}
	if !drawable {
		embeds = append(embeds, imageEmbed("-# No finished games to chart yet!", ""))
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: embeds, AllowedMentions: &discordgo.MessageAllowedMentions{}},
	}); err != nil || !drawable {
		return err
	}

	path := comparePath(bcp, guild, left, right, period, last)
	req := dailygen.CompareRequest{Guild: guild, Name: filepath.Base(path), First: first, Last: last, Window: window, Left: l, Right: r}
	stale := filepath.Join(filepath.Dir(path), comparePrefix(left, right, period)+"_*")
	return attachCard(s, i, head, path, "compare-"+left+"-"+right, stale, "compare image", func() error {
		return compareImage(s, path, guild, left, right, req)
	})
}

// a /compare card's (or link's) pair, from its name: compare_<left>_<right>_...
func comparePair(name string) (left, right string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(name, "compare_"), "_")
	if !strings.HasPrefix(name, "compare_") || len(parts) < 4 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// removes a guild's cached /compare cards and links: every pair user is in,
// or every pair when user is empty, so they're drawn fresh. a pair being
// drawn right now is left alone (busy, by its two sides); cards is how many
// were removed
func clearCompares(bcp *types.BotCommandPackage, guild, user string) (cards int, busy [][2]string, err error) {
	dir := compareDir(bcp, guild)
	files, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return 0, nil, err
	}
	pairs := map[[2]string][]string{}
	for _, f := range files {
		left, right, ok := comparePair(f.Name())
		if !ok || (user != "" && user != left && user != right) {
			continue
		}
		pairs[[2]string{left, right}] = append(pairs[[2]string{left, right}], f.Name())
	}
	for pair, names := range pairs {
		// held while removing, so no draw starts writing beside it
		_, done, claimErr := claimDraw(guild, "compare-"+pair[0]+"-"+pair[1])
		if claimErr != nil {
			busy = append(busy, pair)
			continue
		}
		for _, name := range names {
			if err = os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
				done()
				return cards, busy, err
			}
			if strings.HasSuffix(name, ".webp") {
				cards++
			}
		}
		done()
	}
	return cards, busy, nil
}

// clears and says how it went; whose is how the reply names them. replies
// are embeds, so mentions never ping
func clearCompare(bcp *types.BotCommandPackage, guild, user, whose string) (string, error) {
	cards, busy, err := clearCompares(bcp, guild, user)
	if err != nil {
		return "", err
	}
	text := fmt.Sprintf("Cleared %d cached /compare card%s for %s.", cards, plural(cards), whose)
	for _, pair := range busy {
		right := "this server"
		if pair[1] != GUILD_SIDE {
			right = fmt.Sprintf("<@%s>", pair[1])
		}
		text += fmt.Sprintf("\n-# Skipped <@%s> vs %s: a card is being drawn right now.", pair[0], right)
	}
	return text, nil
}

func registerCompare(bcp *types.BotCommandPackage) {
	periods := make([]*discordgo.ApplicationCommandOptionChoice, len(MEDIAN_PERIODS))
	for j, p := range MEDIAN_PERIODS {
		periods[j] = &discordgo.ApplicationCommandOptionChoice{Name: leaderboard.PERIOD_LABELS[p], Value: p}
	}
	(*bcp.Handles)["compare"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "compare",
			Description: "two players side by side (default: you against this server)",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user1",
				Description: "one player (default: you)",
			}, {
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user2",
				Description: "the other (default: you, or this server if neither is given)",
			}, {
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "since",
				Description: "how far back to count (default: all-time)",
				Choices:     periods,
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.DEFAULT},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			left, right, ok := pairOf(i.Member.User.ID, option(i, "user1", ""), option(i, "user2", ""))
			if !ok {
				return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Pick someone other than themselves to compare against.",
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			}
			return openCompare(s, bcp, i, left, right, option(i, "since", leaderboard.ALLTIME))
		},
	}
	(*bcp.Handles)["mg-clearcompare"] = CommandInfo{
		DGInfo: &discordgo.ApplicationCommand{
			Name:        "mg-clearcompare",
			Description: "delete cached /compare cards so they're drawn fresh",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "every pair they're in (default: every pair in this server)",
			}},
		},
		Metadata: types.CommandMetadata{MinTrustLevel: authorize.MANAGE},
		Callback: func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
			if user := option(i, "user", ""); user != "" {
				text, err := clearCompare(bcp, i.GuildID, user, fmt.Sprintf("<@%s>", user))
				if err != nil {
					return err
				}
				return confirm.Reply(s, i, text)
			}
			// everyone's: ask first
			return confirm.Open(s, i, confirm.Prompt{
				Flow: CLEAR_ALL,
				Question: func(int) string {
					return "Are you sure you want to clear every cached /compare card in this server?"
				},
				Cancelled: "Cancelled, nothing was cleared.",
				TimedOut:  "Time's up, nothing was cleared.",
				Confirm: func() (string, error) {
					text, err := clearCompare(bcp, i.GuildID, "", "this server")
					if err != nil {
						return "Couldn't clear them, try again later.", err
					}
					return text, nil
				},
			})
		},
	}
}
