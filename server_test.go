package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const goodMatch = `{"id":"m1","players":[{"player_id":"alice","score":30,"won":true},{"player_id":"bob","score":20,"won":false}]}`

func TestCreateMatchStatusCodes(t *testing.T) {
	h := NewServer(NewStore()).Handler()
	cases := []struct {
		name, body string
		want       int
	}{
		{"created", goodMatch, http.StatusCreated},
		{"duplicate", goodMatch, http.StatusConflict},
		{"bad json", `{`, http.StatusBadRequest},
		{"unknown field", `{"id":"x","oops":1}`, http.StatusBadRequest},
		{"invalid match", `{"id":"y","players":[{"player_id":"a"}]}`, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := do(t, h, "POST", "/matches", c.body).Code; got != c.want {
				t.Fatalf("want %d, got %d", c.want, got)
			}
		})
	}
}

func TestStatsEndpoint(t *testing.T) {
	h := NewServer(NewStore()).Handler()
	do(t, h, "POST", "/matches", goodMatch)

	rec := do(t, h, "GET", "/players/alice/stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var st Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Wins != 1 || st.TotalScore != 30 {
		t.Fatalf("unexpected stats: %+v", st)
	}
	if got := do(t, h, "GET", "/players/nobody/stats", "").Code; got != http.StatusNotFound {
		t.Fatalf("want 404, got %d", got)
	}
}

func TestLeaderboardEndpoint(t *testing.T) {
	h := NewServer(NewStore()).Handler()
	do(t, h, "POST", "/matches", goodMatch)

	rec := do(t, h, "GET", "/leaderboard?limit=1", "")
	var lb []Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &lb); err != nil {
		t.Fatal(err)
	}
	if len(lb) != 1 || lb[0].PlayerID != "alice" {
		t.Fatalf("unexpected leaderboard: %+v", lb)
	}
	for _, bad := range []string{"0", "101", "abc"} {
		if got := do(t, h, "GET", "/leaderboard?limit="+bad, "").Code; got != http.StatusBadRequest {
			t.Fatalf("limit=%s: want 400, got %d", bad, got)
		}
	}
}

func TestHealthAndMetrics(t *testing.T) {
	h := NewServer(NewStore()).Handler()
	if got := do(t, h, "GET", "/healthz", "").Code; got != http.StatusOK {
		t.Fatalf("healthz: %d", got)
	}
	do(t, h, "POST", "/matches", goodMatch)
	body := do(t, h, "GET", "/metrics", "").Body.String()
	for _, want := range []string{
		`http_requests_total{route="GET /healthz",status="200"} 1`,
		`http_requests_total{route="POST /matches",status="201"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q in:\n%s", want, body)
		}
	}
}
