package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

// A HS256 key shorter than the hash it feeds weakens the signature.
const minSecretLength = 32

type config struct {
	addr     string
	dbPath   string
	secret   string
	tokenTTL time.Duration
}

func loadConfig(logger *slog.Logger) (*config, error) {
	conf := &config{}

	addr := os.Getenv("ADDR")
	if addr == "" {
		logger.Info("No ADDR variable set. Defaulting to :8080")
		addr = ":8080"
	}
	conf.addr = addr

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		logger.Info("No DB_PATH variable set. Defaulting to gotic.db")
		dbPath = "gotic.db"
	}
	conf.dbPath = dbPath

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET shouldn't be empty")
	}

	if len(secret) < minSecretLength {
		return nil, fmt.Errorf("JWT_SECRET should be at least %d bytes", minSecretLength)
	}
	conf.secret = secret

	rawTTL := os.Getenv("TOKEN_TTL")
	if rawTTL == "" {
		logger.Info("No TOKEN_TTL is set. Defaulting to 24 hours")
		conf.tokenTTL = time.Hour * 24
	} else {
		ttl, err := time.ParseDuration(rawTTL)
		if err != nil {
			logger.Warn(err.Error())
			return nil, fmt.Errorf("GOTIC_TOKEN_TTL should be a duration like 24h or 30m")
		}

		conf.tokenTTL = ttl
	}

	return conf, nil
}
