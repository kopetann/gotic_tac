package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kopetann/gotic_tac/internal/adapter/auth"
	"github.com/kopetann/gotic_tac/internal/adapter/id"
	"github.com/kopetann/gotic_tac/internal/adapter/memory"
	"github.com/kopetann/gotic_tac/internal/adapter/transport"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

const testSecret = "transport-test-secret-32-bytes!!!"

// server wires the real stack against in-memory stores: the handlers are the
// only thing under test, so nothing here is faked.
func server(t *testing.T) *httptest.Server {
	t.Helper()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	players := memory.NewPlayerStore()
	matches := memory.NewMatchStore()

	tokens, err := auth.NewAuthProvider(testSecret, time.Hour)
	if err != nil {
		t.Fatalf("Failed to build AuthProvider: %s", err.Error())
	}

	hub := transport.NewHub(log)
	games := usecase.NewGames(matches, players, hub, id.NewUUID(), log)

	srv := httptest.NewServer(transport.NewRouter(transport.Deps{
		Auth:        usecase.NewAuth(players, auth.HashProvider{}, tokens, id.NewUUID()),
		Games:       games,
		Matchmaking: usecase.NewMatchmaking(games),
		Leaderboard: usecase.NewLeaderboard(players),
		Hub:         hub,
		Log:         log,
	}))

	t.Cleanup(srv.Close)

	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path, token string, body any) (int, map[string]any) {
	t.Helper()

	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to encode a body: %s", err.Error())
		}

		payload = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, srv.URL+path, payload)
	if err != nil {
		t.Fatalf("Failed to build a request: %s", err.Error())
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send a request: %s", err.Error())
	}
	defer resp.Body.Close()

	out := map[string]any{}
	json.NewDecoder(resp.Body).Decode(&out)

	return resp.StatusCode, out
}

func registerPlayer(t *testing.T, srv *httptest.Server, name string) string {
	t.Helper()

	status, body := do(t, srv, http.MethodPost, "/register", "", map[string]string{
		"name":     name,
		"password": "password123",
	})

	if status != http.StatusCreated {
		t.Fatalf("Register %s returned %d, want 201", name, status)
	}

	token, ok := body["token"].(string)
	if !ok || token == "" {
		t.Fatalf("Register %s returned no token", name)
	}

	return token
}

func TestHealthz(t *testing.T) {
	srv := server(t)

	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("Failed to reach /healthz: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz = %d, want 200", resp.StatusCode)
	}
}

func TestRegisterAndLogin(t *testing.T) {
	srv := server(t)

	registerPlayer(t, srv, "alice")

	tests := []struct {
		name   string
		path   string
		body   map[string]string
		status int
		code   string
	}{
		{"duplicate name", "/register", map[string]string{"name": "alice", "password": "password123"}, http.StatusConflict, "NAME_TAKEN"},
		{"short password", "/register", map[string]string{"name": "bob", "password": "short"}, http.StatusBadRequest, "PASSWORD_TOO_SHORT"},
		{"empty name", "/register", map[string]string{"name": " ", "password": "password123"}, http.StatusBadRequest, "EMPTY_NAME"},
		{"wrong password", "/login", map[string]string{"name": "alice", "password": "nope12345"}, http.StatusUnauthorized, "INVALID_CREDENTIALS"},
		{"unknown player", "/login", map[string]string{"name": "nobody", "password": "password123"}, http.StatusUnauthorized, "INVALID_CREDENTIALS"},
		{"correct password", "/login", map[string]string{"name": "alice", "password": "password123"}, http.StatusOK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, srv, http.MethodPost, tt.path, "", tt.body)

			if status != tt.status {
				t.Fatalf("%s %s = %d, want %d (%v)", http.MethodPost, tt.path, status, tt.status, body)
			}

			if tt.code != "" && body["code"] != tt.code {
				t.Errorf("code = %v, want %q", body["code"], tt.code)
			}
		})
	}
}

// An unknown name and a wrong password must be indistinguishable, or the
// endpoint tells an attacker which accounts exist.
func TestLoginDoesNotLeakExistingNames(t *testing.T) {
	srv := server(t)
	registerPlayer(t, srv, "alice")

	_, unknown := do(t, srv, http.MethodPost, "/login", "", map[string]string{"name": "nobody", "password": "password123"})
	_, wrong := do(t, srv, http.MethodPost, "/login", "", map[string]string{"name": "alice", "password": "nope12345"})

	if unknown["code"] != wrong["code"] || unknown["message"] != wrong["message"] {
		t.Errorf("unknown name gave %v, wrong password gave %v", unknown, wrong)
	}
}

func TestProtectedRoutesRejectBadTokens(t *testing.T) {
	srv := server(t)

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/match"},
		{http.MethodPost, "/match/move"},
		{http.MethodPost, "/match/resign"},
		{http.MethodPost, "/matchmaking/join"},
		{http.MethodPost, "/matchmaking/leave"},
	}

	tokens := map[string]string{
		"no token":      "",
		"garbage token": "not-a-token",
	}

	for _, p := range paths {
		for name, token := range tokens {
			t.Run(p.path+" with "+name, func(t *testing.T) {
				status, _ := do(t, srv, p.method, p.path, token, map[string]int{"cell": 0})

				if status != http.StatusUnauthorized {
					t.Errorf("%s %s = %d, want 401", p.method, p.path, status)
				}
			})
		}
	}
}

func TestMatchFlow(t *testing.T) {
	srv := server(t)

	alice := registerPlayer(t, srv, "alice")
	bob := registerPlayer(t, srv, "bob")

	if status, _ := do(t, srv, http.MethodGet, "/match", alice, nil); status != http.StatusNotFound {
		t.Fatalf("GET /match before a match = %d, want 404", status)
	}

	if status, _ := do(t, srv, http.MethodPost, "/matchmaking/join", alice, nil); status != http.StatusAccepted {
		t.Fatalf("alice join = %d, want 202", status)
	}

	if status, body := do(t, srv, http.MethodPost, "/matchmaking/join", alice, nil); status != http.StatusConflict {
		t.Fatalf("alice joining twice = %d (%v), want 409", status, body)
	}

	if status, _ := do(t, srv, http.MethodPost, "/matchmaking/join", bob, nil); status != http.StatusAccepted {
		t.Fatalf("bob join = %d, want 202", status)
	}

	status, state := do(t, srv, http.MethodGet, "/match", alice, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /match after pairing = %d, want 200", status)
	}

	if state["your_mark"] != "X" || state["opponent"] != "bob" {
		t.Fatalf("alice sees %v, want mark X against bob", state)
	}

	// Bob is O, so moving first is a turn violation rather than a bad cell.
	if status, body := do(t, srv, http.MethodPost, "/match/move", bob, map[string]int{"cell": 0}); status != http.StatusConflict || body["code"] != "NOT_YOUR_TURN" {
		t.Fatalf("bob moving first = %d %v, want 409 NOT_YOUR_TURN", status, body)
	}

	// X takes the top row while O answers underneath.
	moves := []struct {
		token string
		cell  int
	}{
		{alice, 0}, {bob, 3}, {alice, 1}, {bob, 4}, {alice, 2},
	}

	var last map[string]any
	for _, m := range moves {
		status, body := do(t, srv, http.MethodPost, "/match/move", m.token, map[string]int{"cell": m.cell})
		if status != http.StatusOK {
			t.Fatalf("move %d = %d (%v), want 200", m.cell, status, body)
		}

		last = body
	}

	if last["status"] != "x_won" || last["outcome"] != "win" || last["finished"] != true {
		t.Fatalf("final state = %v, want x_won / win / finished", last)
	}

	// A finished match is dropped, so both players are free again.
	if status, _ := do(t, srv, http.MethodGet, "/match", alice, nil); status != http.StatusNotFound {
		t.Errorf("GET /match after the win = %d, want 404", status)
	}

	status, board := do(t, srv, http.MethodGet, "/leaderboard?limit=5", "", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /leaderboard = %d, want 200", status)
	}

	rows, _ := board["standings"].([]any)
	if len(rows) != 2 {
		t.Fatalf("leaderboard has %d rows, want 2", len(rows))
	}

	top, _ := rows[0].(map[string]any)
	if top["name"] != "alice" || top["wins"] != float64(1) {
		t.Errorf("top of the leaderboard = %v, want alice with 1 win", top)
	}
}

func TestMoveRejectsBadCells(t *testing.T) {
	srv := server(t)

	alice := registerPlayer(t, srv, "alice")
	bob := registerPlayer(t, srv, "bob")

	do(t, srv, http.MethodPost, "/matchmaking/join", alice, nil)
	do(t, srv, http.MethodPost, "/matchmaking/join", bob, nil)
	do(t, srv, http.MethodPost, "/match/move", alice, map[string]int{"cell": 4})

	tests := []struct {
		name   string
		token  string
		cell   int
		status int
		code   string
	}{
		{"negative", bob, -1, http.StatusBadRequest, "CELL_OUT_OF_BOUNDS"},
		{"past the end", bob, 9, http.StatusBadRequest, "CELL_OUT_OF_BOUNDS"},
		{"occupied", bob, 4, http.StatusConflict, "CELL_OCCUPIED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, srv, http.MethodPost, "/match/move", tt.token, map[string]int{"cell": tt.cell})

			if status != tt.status || body["code"] != tt.code {
				t.Errorf("move %d = %d %v, want %d %s", tt.cell, status, body, tt.status, tt.code)
			}
		})
	}
}

func TestMalformedBody(t *testing.T) {
	srv := server(t)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/register", bytes.NewReader([]byte("{not json")))
	if err != nil {
		t.Fatalf("Failed to build a request: %s", err.Error())
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Failed to send a request: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /register with broken JSON = %d, want 400", resp.StatusCode)
	}
}

// A failing Health func must take the endpoint down, otherwise the container
// reports healthy while its database is gone.
func TestHealthzFailsWhenDatabaseIsDown(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv := httptest.NewServer(transport.NewRouter(transport.Deps{
		Log:    log,
		Health: func(context.Context) error { return context.DeadlineExceeded },
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("Failed to reach /healthz: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("GET /healthz = %d, want 503", resp.StatusCode)
	}
}
