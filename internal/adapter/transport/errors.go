package transport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

// Transport level failures. They are domain.Error so every error on the wire
// carries a stable code, whatever layer produced it.
var (
	errMalformedBody = &domain.Error{Code: "MALFORMED_BODY", Message: "request body is not valid JSON"}
	errMissingToken  = &domain.Error{Code: "MISSING_TOKEN", Message: "authorization header must be a bearer token"}
	errUnknownCmd    = &domain.Error{Code: "UNKNOWN_COMMAND", Message: "unknown command"}
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// classify decides what a client is allowed to see. A *domain.Error was
// written for a user, so it travels as is. Anything else is a bug or a broken
// dependency and collapses to a generic 500.
func classify(err error) (int, errorResponse) {
	var de *domain.Error
	if !errors.As(err, &de) {
		return http.StatusInternalServerError, errorResponse{
			Code:    "INTERNAL",
			Message: "internal error",
		}
	}

	return statusFor(de.Code), errorResponse{Code: de.Code, Message: de.Message}
}

func statusFor(code string) int {
	switch code {
	case usecase.ErrInvalidToken.Code, usecase.ErrInvalidCredentials.Code, errMissingToken.Code:
		return http.StatusUnauthorized

	case usecase.ErrNameTaken.Code:
		return http.StatusConflict

	case usecase.ErrPlayerNotFound.Code, usecase.ErrNoActiveMatch.Code:
		return http.StatusNotFound

	case usecase.ErrAlreadyQueued.Code, usecase.ErrAlreadyInMatch.Code, domain.ErrNotYourTurn.Code,
		domain.ErrCellOccupied.Code, domain.ErrMatchFinished.Code:
		return http.StatusConflict

	default:
		return http.StatusBadRequest
	}
}

func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Warn("Failed to write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, log *slog.Logger, err error) {
	status, body := classify(err)
	if status == http.StatusInternalServerError {
		log.Error("Unexpected failure", "error", err)
	}

	writeJSON(w, log, status, body)
}

func errorFrame(err error) frame {
	_, body := classify(err)

	return frame{Type: frameError, Code: body.Code, Message: body.Message}
}
