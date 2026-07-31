package transport

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/kopetann/gotic_tac/internal/usecase"
)

// Deps is a struct rather than a parameter list so adding a handler later does
// not ripple a signature change out into main.
type Deps struct {
	Auth        *usecase.Auth
	Games       *usecase.Games
	Matchmaking *usecase.Matchmaking
	Leaderboard *usecase.Leaderboard
	Hub         *Hub
	Log         *slog.Logger

	// Health is optional and keeps database/sql out of this package.
	Health func(ctx context.Context) error
}

var _ usecase.Notifier = (*Hub)(nil)

// NewRouter maps the protocol onto the use cases. Go 1.22 patterns carry the
// method, so no third party router is needed.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", d.handleHealth)

	mux.HandleFunc("POST /register", d.handleRegister)
	mux.HandleFunc("POST /login", d.handleLogin)
	mux.HandleFunc("GET /leaderboard", d.handleLeaderboard)

	mux.HandleFunc("POST /matchmaking/join", d.authenticated(d.handleJoin))
	mux.HandleFunc("POST /matchmaking/leave", d.authenticated(d.handleLeave))

	mux.HandleFunc("GET /match", d.authenticated(d.handleState))
	mux.HandleFunc("POST /match/move", d.authenticated(d.handleMove))
	mux.HandleFunc("POST /match/resign", d.authenticated(d.handleResign))

	mux.HandleFunc("GET /ws", d.handleWS)

	return d.logRequests(mux)
}

func (d Deps) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.Log.Debug("Request", "method", r.Method, "path", r.URL.Path)

		next.ServeHTTP(w, r)
	})
}
