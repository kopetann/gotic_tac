package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kopetann/gotic_tac/internal/adapter/auth"
	"github.com/kopetann/gotic_tac/internal/adapter/id"
	"github.com/kopetann/gotic_tac/internal/adapter/sqlite"
	"github.com/kopetann/gotic_tac/internal/adapter/transport"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
func run() error {
	// Global ctx
	ctx := context.Background()

	// Watch for the cancellation/termination signals
	cancelCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()

	// Initialize a logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Environment variables
	env, err := loadConfig(logger)
	if err != nil {
		return err
	}

	// SQLite DB init
	db, err := sqlite.Open(ctx, env.dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	logger.Info("Database is ready", "path", env.dbPath)

	// Adapters
	players := sqlite.NewPlayerStore(db)
	matches := sqlite.NewMatchStore(db)
	hasher := auth.HashProvider{}
	ids := id.NewUUID()

	tokens, err := auth.NewAuthProvider(env.secret, env.tokenTTL)
	if err != nil {
		return err
	}

	// The hub is built before Games because Games takes it as its Notifier.
	// It holds no sockets until the first client upgrades.
	hub := transport.NewHub(logger)

	// Dependencies init
	games := usecase.NewGames(matches, players, hub, ids, logger)
	deps := transport.Deps{
		Auth:        usecase.NewAuth(players, hasher, tokens, ids),
		Games:       games,
		Matchmaking: usecase.NewMatchmaking(games),
		Leaderboard: usecase.NewLeaderboard(players),
		Hub:         hub,
		Log:         logger,
		Health:      db.PingContext,
	}

	// HTTP server
	srv := http.Server{
		Handler:           transport.NewRouter(deps),
		Addr:              env.addr,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("Server started", "addr", env.addr)

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(err.Error())
			stop()
		}
	}()

	<-cancelCtx.Done()

	logger.Info("Stopping a server...")

	shutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return srv.Shutdown(shutCtx)
}
