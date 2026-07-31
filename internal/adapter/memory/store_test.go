package memory_test

import (
	"testing"

	"github.com/kopetann/gotic_tac/internal/adapter/memory"
	"github.com/kopetann/gotic_tac/internal/adapter/storetest"
)

func TestPlayerStore(t *testing.T) {
	storetest.RunPlayerStore(t, func(t *testing.T) storetest.PlayerStore {
		return memory.NewPlayerStore()
	})
}

func TestMatchStore(t *testing.T) {
	storetest.RunMatchStore(t, func(t *testing.T) storetest.MatchStore {
		return memory.NewMatchStore()
	})
}
