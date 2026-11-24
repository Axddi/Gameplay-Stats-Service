package main

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func twoPlayerMatch(id string) Match {
	return Match{ID: id, Players: []PlayerResult{
		{PlayerID: "alice", Score: 30, Won: true},
		{PlayerID: "bob", Score: 20, Won: false},
	}}
}

func TestRecordUpdatesStats(t *testing.T) {
	s := NewStore()
	if err := s.Record(twoPlayerMatch("m1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(twoPlayerMatch("m2")); err != nil {
		t.Fatal(err)
	}
	st, ok := s.PlayerStats("alice")
	if !ok {
		t.Fatal("alice not found")
	}
	if st.Matches != 2 || st.Wins != 2 || st.TotalScore != 60 || st.WinRate != 1 {
		t.Fatalf("unexpected stats: %+v", st)
	}
	bob, _ := s.PlayerStats("bob")
	if bob.Wins != 0 || bob.WinRate != 0 {
		t.Fatalf("unexpected stats: %+v", bob)
	}
}

func TestRecordValidation(t *testing.T) {
	cases := map[string]Match{
		"no id":          {Players: twoPlayerMatch("x").Players},
		"one player":     {ID: "m", Players: []PlayerResult{{PlayerID: "a"}}},
		"empty player":   {ID: "m", Players: []PlayerResult{{PlayerID: ""}, {PlayerID: "b"}}},
		"negative score": {ID: "m", Players: []PlayerResult{{PlayerID: "a", Score: -1}, {PlayerID: "b"}}},
		"duplicate":      {ID: "m", Players: []PlayerResult{{PlayerID: "a"}, {PlayerID: "a"}}},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if err := NewStore().Record(m); !errors.Is(err, ErrInvalidMatch) {
				t.Fatalf("want ErrInvalidMatch, got %v", err)
			}
		})
	}
}

func TestDuplicateMatchIsRejectedAndNotCounted(t *testing.T) {
	s := NewStore()
	_ = s.Record(twoPlayerMatch("m1"))
	if err := s.Record(twoPlayerMatch("m1")); !errors.Is(err, ErrDuplicateMatch) {
		t.Fatalf("want ErrDuplicateMatch, got %v", err)
	}
	st, _ := s.PlayerStats("alice")
	if st.Matches != 1 {
		t.Fatalf("retry double-counted: %+v", st)
	}
}

func TestLeaderboardOrderAndLimit(t *testing.T) {
	s := NewStore()
	_ = s.Record(Match{ID: "1", Players: []PlayerResult{{PlayerID: "c", Score: 5, Won: true}, {PlayerID: "a", Score: 9}}})
	_ = s.Record(Match{ID: "2", Players: []PlayerResult{{PlayerID: "b", Score: 7, Won: true}, {PlayerID: "a", Score: 1}}})
	_ = s.Record(Match{ID: "3", Players: []PlayerResult{{PlayerID: "d", Score: 0}, {PlayerID: "e", Score: 0}}})

	lb := s.Leaderboard(0)
	want := []string{"b", "c", "a", "d", "e"}
	for i, id := range want {
		if lb[i].PlayerID != id {
			t.Fatalf("position %d: want %s, got %s (%v)", i, id, lb[i].PlayerID, lb)
		}
	}
	if got := len(s.Leaderboard(2)); got != 2 {
		t.Fatalf("limit not applied: %d", got)
	}
}

func TestConcurrentRecord(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	const n = 200
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Record(twoPlayerMatch(fmt.Sprintf("m%d", i)))
			_ = s.Leaderboard(5)
		}(i)
	}
	wg.Wait()
	st, _ := s.PlayerStats("alice")
	if st.Matches != n {
		t.Fatalf("lost updates: want %d, got %d", n, st.Matches)
	}
}
