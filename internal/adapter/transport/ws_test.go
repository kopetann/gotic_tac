package transport_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

func dial(t *testing.T, url, token string) *websocket.Conn {
	t.Helper()

	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}

	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}

		t.Fatalf("Failed to dial the socket: %s (status %d)", err.Error(), status)
	}

	t.Cleanup(func() { conn.Close() })

	return conn
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + "/ws"
}

func send(t *testing.T, conn *websocket.Conn, cmd map[string]any) {
	t.Helper()

	if err := conn.WriteJSON(cmd); err != nil {
		t.Fatalf("Failed to send a command: %s", err.Error())
	}
}

// joinQueue waits for the ack, which fixes the queue order: two sockets are
// served by two goroutines, so without a barrier X could go to either player.
// Only the first player needs it, because the second one's join pairs
// immediately and its match_started arrives ahead of its ack.
func joinQueue(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	send(t, conn, map[string]any{"type": "join"})
	nextOfType(t, conn, "ack")
}

// nextOfType skips acks and unrelated frames so a test can wait for the one
// event it cares about without depending on delivery order.
func nextOfType(t *testing.T, conn *websocket.Conn, want string) map[string]any {
	t.Helper()

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	for range 10 {
		var f map[string]any
		if err := conn.ReadJSON(&f); err != nil {
			t.Fatalf("Failed to read a %q frame: %s", want, err.Error())
		}

		if f["type"] == want {
			return f
		}
	}

	t.Fatalf("Never received a %q frame", want)

	return nil
}

func TestWSRejectsBadToken(t *testing.T) {
	srv := server(t)

	tokens := map[string]string{
		"no token":      "",
		"garbage token": "not-a-token",
	}

	for name, token := range tokens {
		t.Run(name, func(t *testing.T) {
			header := http.Header{}
			if token != "" {
				header.Set("Authorization", "Bearer "+token)
			}

			conn, resp, err := websocket.DefaultDialer.Dial(wsURL(srv.URL), header)
			if err == nil {
				conn.Close()
				t.Fatal("Dial succeeded, want a rejected handshake")
			}

			if resp == nil || resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("Handshake status = %v, want 401", resp)
			}
		})
	}
}

func TestWSMatchIsPushedToBothPlayers(t *testing.T) {
	srv := server(t)

	alice := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "alice"))
	bob := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "bob"))

	joinQueue(t, alice)
	send(t, bob, map[string]any{"type": "join"})

	aliceStart := nextOfType(t, alice, string(usecase.EventMatchStarted))
	bobStart := nextOfType(t, bob, string(usecase.EventMatchStarted))

	if aliceStart["match_id"] != bobStart["match_id"] {
		t.Fatalf("players were put in different matches: %v vs %v", aliceStart["match_id"], bobStart["match_id"])
	}

	// Each player is told their own mark, never the other's.
	aliceState, _ := aliceStart["state"].(map[string]any)
	bobState, _ := bobStart["state"].(map[string]any)

	if aliceState["your_mark"] != "X" || bobState["your_mark"] != "O" {
		t.Fatalf("marks = %v and %v, want X and O", aliceState["your_mark"], bobState["your_mark"])
	}

	if aliceState["opponent"] != "bob" || bobState["opponent"] != "alice" {
		t.Errorf("opponents = %v and %v", aliceState["opponent"], bobState["opponent"])
	}
}

func TestWSMoveNotifiesTheOpponent(t *testing.T) {
	srv := server(t)

	alice := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "alice"))
	bob := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "bob"))

	joinQueue(t, alice)
	send(t, bob, map[string]any{"type": "join"})

	nextOfType(t, alice, string(usecase.EventMatchStarted))
	nextOfType(t, bob, string(usecase.EventMatchStarted))

	send(t, alice, map[string]any{"type": "move", "cell": 4})

	changed := nextOfType(t, bob, string(usecase.EventStateChanged))
	state, _ := changed["state"].(map[string]any)
	cells, _ := state["cells"].([]any)

	if cells[4] != "X" {
		t.Errorf("bob sees cell 4 = %v, want X", cells[4])
	}

	if state["turn"] != "O" {
		t.Errorf("turn = %v, want O", state["turn"])
	}
}

func TestWSRejectsOutOfTurnMove(t *testing.T) {
	srv := server(t)

	alice := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "alice"))
	bob := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "bob"))

	joinQueue(t, alice)
	send(t, bob, map[string]any{"type": "join"})

	nextOfType(t, alice, string(usecase.EventMatchStarted))
	nextOfType(t, bob, string(usecase.EventMatchStarted))

	send(t, bob, map[string]any{"type": "move", "cell": 0})

	f := nextOfType(t, bob, "error")
	if f["code"] != "NOT_YOUR_TURN" {
		t.Errorf("code = %v, want NOT_YOUR_TURN", f["code"])
	}
}

func TestWSUnknownCommand(t *testing.T) {
	srv := server(t)

	conn := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "alice"))
	send(t, conn, map[string]any{"type": "fly"})

	f := nextOfType(t, conn, "error")
	if f["code"] != "UNKNOWN_COMMAND" {
		t.Errorf("code = %v, want UNKNOWN_COMMAND", f["code"])
	}
}

func TestWSDisconnectEndsTheOpponentsMatch(t *testing.T) {
	srv := server(t)

	alice := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "alice"))
	bob := dial(t, wsURL(srv.URL), registerPlayer(t, srv, "bob"))

	joinQueue(t, alice)
	send(t, bob, map[string]any{"type": "join"})

	nextOfType(t, alice, string(usecase.EventMatchStarted))
	nextOfType(t, bob, string(usecase.EventMatchStarted))

	alice.Close()

	f := nextOfType(t, bob, string(usecase.EventOpponentLeft))
	state, _ := f["state"].(map[string]any)

	if state == nil || state["your_mark"] != "O" {
		t.Errorf("opponent_left carried %v, want bob's own state", f["state"])
	}
}

// A second socket for the same player must replace the first, or the hub holds
// a connection nothing will ever read from again.
func TestWSReconnectReplacesTheOldSession(t *testing.T) {
	srv := server(t)
	token := registerPlayer(t, srv, "alice")

	first := dial(t, wsURL(srv.URL), token)
	second := dial(t, wsURL(srv.URL), token)

	// The newest socket is the live one.
	send(t, second, map[string]any{"type": "state"})
	nextOfType(t, second, "error")

	// The old one is closed by the server rather than left hanging.
	first.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := first.ReadMessage(); err == nil {
		t.Error("The replaced socket is still readable, want it closed")
	}
}
