package scoredb

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestGuildAndPlayersExclude(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "scores.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint
	rows := []Row{
		{GuildID: "g", UserID: "a", Day: 1, Emojis: "x"},
		{GuildID: "g", UserID: "a", Day: 2, Emojis: "x"},
		{GuildID: "g", UserID: "b", Day: 1, Emojis: "x"},
		{GuildID: "g", UserID: "c", Day: 1, Emojis: "x"},
		{GuildID: "other", UserID: "d", Day: 1, Emojis: "x"},
	}
	if err := db.UpsertMany(rows); err != nil {
		t.Fatal(err)
	}
	all, _ := db.Guild("g", nil, 1)
	if len(all) != 4 {
		t.Fatalf("Guild(no exclude) = %d rows", len(all))
	}
	some, err := db.Guild("g", []string{"a", "c"}, 1)
	if err != nil || len(some) != 1 || some[0].UserID != "b" {
		t.Fatalf("Guild(exclude a,c) = %+v %v", some, err)
	}
	ids, err := db.Players("g", []string{"b"}, 1)
	slices.Sort(ids)
	if err != nil || !slices.Equal(ids, []string{"a", "c"}) {
		t.Fatalf("Players(exclude b) = %v %v", ids, err)
	}
	// only day 2 on: just a's second game
	if recent, _ := db.Guild("g", nil, 2); len(recent) != 1 || recent[0].UserID != "a" || recent[0].Day != 2 {
		t.Fatalf("Guild(from day 2) = %+v", recent)
	}
	if ids, _ := db.Players("g", nil, 2); len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("Players(from day 2) = %v", ids)
	}
}
