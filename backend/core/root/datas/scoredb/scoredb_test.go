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

func TestCountAndDelete(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "scores.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint
	if err := db.UpsertMany([]Row{
		{GuildID: "g", UserID: "a", Day: 1, Emojis: "x"},
		{GuildID: "g", UserID: "a", Day: 2, Emojis: "x"},
		{GuildID: "g", UserID: "a", Day: 3, Emojis: "x"},
		{GuildID: "g", UserID: "b", Day: 1, Emojis: "x"},
		{GuildID: "other", UserID: "a", Day: 1, Emojis: "x"},
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := db.CountUser("g", "a"); err != nil || n != 3 {
		t.Fatalf("CountUser = %d, %v", n, err)
	}
	if ok, err := db.Delete("g", "a", 2); err != nil || !ok {
		t.Fatalf("Delete existing = %v, %v", ok, err)
	}
	if ok, err := db.Delete("g", "a", 2); err != nil || ok {
		t.Fatalf("Delete missing = %v, %v", ok, err)
	}
	if n, err := db.DeleteUser("g", "a"); err != nil || n != 2 {
		t.Fatalf("DeleteUser = %d, %v", n, err)
	}
	// only that user, only that guild
	if n, _ := db.CountUser("g", "a"); n != 0 {
		t.Errorf("a still has %d in g", n)
	}
	if n, _ := db.CountUser("g", "b"); n != 1 {
		t.Errorf("b lost results: %d", n)
	}
	if n, _ := db.CountUser("other", "a"); n != 1 {
		t.Errorf("a's other guild lost results: %d", n)
	}
}
