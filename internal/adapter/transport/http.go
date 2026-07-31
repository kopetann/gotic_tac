package transport

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

// playerOf trusts nothing but the token. No handler reads an identity out of a
// request body, so a client can never act as somebody else.
func (d Deps) playerOf(r *http.Request) (domain.PlayerID, error) {
	header := r.Header.Get("Authorization")

	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return "", errMissingToken
	}

	return d.Auth.Authenticate(strings.TrimSpace(token))
}

// authenticated resolves the caller once and hands the id to the handler, so
// no handler has to remember to do it.
func (d Deps) authenticated(next func(http.ResponseWriter, *http.Request, domain.PlayerID)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := d.playerOf(r)
		if err != nil {
			writeError(w, d.Log, err)
			return
		}

		next(w, r, id)
	}
}

func decode(r *http.Request, dst any) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return errMalformedBody
	}

	return nil
}

func (d Deps) handleHealth(w http.ResponseWriter, r *http.Request) {
	if d.Health != nil {
		if err := d.Health(r.Context()); err != nil {
			d.Log.Error("Health check failed", "error", err)
			writeJSON(w, d.Log, http.StatusServiceUnavailable, errorResponse{
				Code:    "UNAVAILABLE",
				Message: "database is not reachable",
			})

			return
		}
	}

	w.Write([]byte("ok"))
}

func (d Deps) handleRegister(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := decode(r, &creds); err != nil {
		writeError(w, d.Log, err)
		return
	}

	token, err := d.Auth.Register(r.Context(), creds.Name, creds.Password)
	if err != nil {
		writeError(w, d.Log, err)
		return
	}

	writeJSON(w, d.Log, http.StatusCreated, tokenResponse{Token: token})
}

func (d Deps) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := decode(r, &creds); err != nil {
		writeError(w, d.Log, err)
		return
	}

	token, err := d.Auth.Login(r.Context(), creds.Name, creds.Password)
	if err != nil {
		writeError(w, d.Log, err)
		return
	}

	writeJSON(w, d.Log, http.StatusOK, tokenResponse{Token: token})
}

func (d Deps) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		limit = usecase.DefaultLeaderboardLimit
	}

	top, err := d.Leaderboard.Top(r.Context(), limit)
	if err != nil {
		writeError(w, d.Log, err)
		return
	}

	body := leaderboardResponse{Standings: make([]standing, 0, len(top))}
	for _, s := range top {
		body.Standings = append(body.Standings, standing{
			Rank:   s.Rank,
			Name:   s.Name,
			Wins:   s.Wins,
			Losses: s.Losses,
			Draws:  s.Draws,
		})
	}

	writeJSON(w, d.Log, http.StatusOK, body)
}

func (d Deps) handleJoin(w http.ResponseWriter, r *http.Request, id domain.PlayerID) {
	if err := d.Matchmaking.Join(r.Context(), id); err != nil {
		writeError(w, d.Log, err)
		return
	}

	writeJSON(w, d.Log, http.StatusAccepted, queueResponse{Waiting: d.Matchmaking.Waiting()})
}

func (d Deps) handleLeave(w http.ResponseWriter, r *http.Request, id domain.PlayerID) {
	d.Matchmaking.Leave(id)

	writeJSON(w, d.Log, http.StatusOK, queueResponse{Waiting: d.Matchmaking.Waiting()})
}

func (d Deps) handleState(w http.ResponseWriter, r *http.Request, id domain.PlayerID) {
	state, err := d.Games.State(id)
	if err != nil {
		writeError(w, d.Log, err)
		return
	}

	writeJSON(w, d.Log, http.StatusOK, newMatchState(state))
}

func (d Deps) handleMove(w http.ResponseWriter, r *http.Request, id domain.PlayerID) {
	var body moveRequest
	if err := decode(r, &body); err != nil {
		writeError(w, d.Log, err)
		return
	}

	state, err := d.Games.Move(r.Context(), id, body.Cell)
	if err != nil {
		writeError(w, d.Log, err)
		return
	}

	writeJSON(w, d.Log, http.StatusOK, newMatchState(state))
}

func (d Deps) handleResign(w http.ResponseWriter, r *http.Request, id domain.PlayerID) {
	if !d.Games.InMatch(id) {
		writeError(w, d.Log, usecase.ErrNoActiveMatch)
		return
	}

	d.Games.Abandon(id)

	w.WriteHeader(http.StatusNoContent)
}
