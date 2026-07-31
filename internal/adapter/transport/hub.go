package transport

import (
	"log/slog"
	"sync"

	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

// Hub holds at most one live socket per player and satisfies usecase.Notifier.
type Hub struct {
	mu      sync.RWMutex
	clients map[domain.PlayerID]*client
	log     *slog.Logger
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[domain.PlayerID]*client),
		log:     log,
	}
}

// Notify must not block, so a full buffer means the peer is gone or too slow
// and gets dropped instead of stalling the opponent's move.
func (h *Hub) Notify(id domain.PlayerID, ev usecase.Event) {
	h.mu.RLock()
	c, ok := h.clients[id]
	h.mu.RUnlock()

	if !ok {
		return
	}

	if !c.push(eventFrame(ev)) {
		h.log.Warn("Dropping a slow client", "player", id)
		c.close()
	}
}

// register replaces any earlier session for the same player. Overwriting the
// map entry without closing the old socket would leak its two goroutines.
func (h *Hub) register(c *client) {
	h.mu.Lock()
	old, ok := h.clients[c.id]
	h.clients[c.id] = c
	h.mu.Unlock()

	if ok {
		h.log.Info("Replacing an existing session", "player", c.id)
		old.close()
	}
}

// unregister is a no-op when the player has already reconnected, so a late
// disconnect cannot evict the newer socket.
func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if current, ok := h.clients[c.id]; ok && current == c {
		delete(h.clients, c.id)
	}
}

func (h *Hub) Online() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients)
}
