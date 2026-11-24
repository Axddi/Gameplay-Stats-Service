package main

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

type PlayerResult struct {
	PlayerID string `json:"player_id"`
	Score    int    `json:"score"`
	Won      bool   `json:"won"`
}

type Match struct {
	ID      string         `json:"id"`
	Players []PlayerResult `json:"players"`
}

type Stats struct {
	PlayerID   string  `json:"player_id"`
	Matches    int     `json:"matches"`
	Wins       int     `json:"wins"`
	TotalScore int     `json:"total_score"`
	WinRate    float64 `json:"win_rate"`
}

var (
	ErrInvalidMatch   = errors.New("invalid match")
	ErrDuplicateMatch = errors.New("match already recorded")
)

type Store struct {
	mu      sync.RWMutex
	matches map[string]struct{}
	players map[string]*Stats
}

func NewStore() *Store {
	return &Store{
		matches: make(map[string]struct{}),
		players: make(map[string]*Stats),
	}
}

func validate(m Match) error {
	if m.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidMatch)
	}
	if len(m.Players) < 2 {
		return fmt.Errorf("%w: at least 2 players are required", ErrInvalidMatch)
	}
	seen := make(map[string]struct{}, len(m.Players))
	for _, p := range m.Players {
		if p.PlayerID == "" {
			return fmt.Errorf("%w: player_id is required", ErrInvalidMatch)
		}
		if p.Score < 0 {
			return fmt.Errorf("%w: score cannot be negative", ErrInvalidMatch)
		}
		if _, dup := seen[p.PlayerID]; dup {
			return fmt.Errorf("%w: player %q appears twice", ErrInvalidMatch, p.PlayerID)
		}
		seen[p.PlayerID] = struct{}{}
	}
	return nil
}

func (s *Store) Record(m Match) error {
	if err := validate(m); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.matches[m.ID]; exists {
		return ErrDuplicateMatch
	}
	s.matches[m.ID] = struct{}{}
	for _, p := range m.Players {
		st, ok := s.players[p.PlayerID]
		if !ok {
			st = &Stats{PlayerID: p.PlayerID}
			s.players[p.PlayerID] = st
		}
		st.Matches++
		st.TotalScore += p.Score
		if p.Won {
			st.Wins++
		}
	}
	return nil
}

func withRate(st Stats) Stats {
	if st.Matches > 0 {
		st.WinRate = float64(st.Wins) / float64(st.Matches)
	}
	return st
}

func (s *Store) PlayerStats(id string) (Stats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.players[id]
	if !ok {
		return Stats{}, false
	}
	return withRate(*st), true
}

func (s *Store) Leaderboard(limit int) []Stats {
	s.mu.RLock()
	all := make([]Stats, 0, len(s.players))
	for _, st := range s.players {
		all = append(all, withRate(*st))
	}
	s.mu.RUnlock()

	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.Wins != b.Wins {
			return a.Wins > b.Wins
		}
		if a.TotalScore != b.TotalScore {
			return a.TotalScore > b.TotalScore
		}
		return a.PlayerID < b.PlayerID
	})
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all
}
