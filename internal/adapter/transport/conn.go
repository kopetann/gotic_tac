package transport

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kopetann/gotic_tac/internal/domain"
	"github.com/kopetann/gotic_tac/internal/usecase"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = pongWait * 9 / 10
	maxMessageSize = 512
	sendBuffer     = 16
)

// CheckOrigin allows every origin: this API is authorised by a bearer token
// rather than a cookie, so there is no ambient authority for another site to abuse.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type client struct {
	id   domain.PlayerID
	conn *websocket.Conn
	send chan frame
	done chan struct{}
	deps Deps
	once sync.Once
}

// close is idempotent because the read pump, the write pump and the hub can all decide a client is finished.
func (c *client) close() {
	c.once.Do(func() {
		close(c.done)
		c.conn.Close()
	})
}

// push never blocks. A full buffer means the peer is gone or too slow, and no
// single client is allowed to stall the goroutine serving their opponent.
func (c *client) push(f frame) bool {
	select {
	case <-c.done:
		return false
	default:
	}

	select {
	case c.send <- f:
		return true
	default:
		return false
	}
}

// handleWS authenticates before upgrading, because after the handshake there is no status code left to answer with.
func (d Deps) handleWS(w http.ResponseWriter, r *http.Request) {
	id, err := d.playerOf(r)
	if err != nil {
		writeError(w, d.Log, err)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		d.Log.Warn("Failed to upgrade a connection", "error", err)
		return
	}

	c := &client{
		id:   id,
		conn: conn,
		send: make(chan frame, sendBuffer),
		done: make(chan struct{}),
		deps: d,
	}

	d.Hub.register(c)

	go c.writePump()
	c.readPump()
}

// writePump is the only goroutine that writes to the socket: a gorilla
// connection supports one concurrent writer and no more.
func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case f := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteJSON(f); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.done:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
	}
}

// readPump runs on the handler goroutine so the request stays alive for as
// long as the socket does.
func (c *client) readPump() {
	defer func() {
		c.deps.Hub.unregister(c)
		c.deps.Matchmaking.Leave(c.id)
		c.deps.Games.Abandon(c.id)
		c.close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// The request context is cancelled at the upgrade, so commands get a fresh one.
	ctx := context.Background()

	for {
		var cmd command
		if err := c.conn.ReadJSON(&cmd); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.deps.Log.Warn("Socket closed unexpectedly", "player", c.id, "error", err)
			}

			return
		}

		f, err := c.dispatch(ctx, cmd)
		if err != nil {
			c.push(errorFrame(err))
			continue
		}

		c.push(f)
	}
}

// dispatch never validates the game. Move already enforces turn order, occupied cells and participation.
func (c *client) dispatch(ctx context.Context, cmd command) (frame, error) {
	switch cmd.Type {
	case cmdJoin:
		if err := c.deps.Matchmaking.Join(ctx, c.id); err != nil {
			return frame{}, err
		}

	case cmdLeave:
		c.deps.Matchmaking.Leave(c.id)

	case cmdMove:
		if err := c.deps.Games.Move(ctx, c.id, cmd.Cell); err != nil {
			return frame{}, err
		}

	case cmdResign:
		if !c.deps.Games.InMatch(c.id) {
			return frame{}, usecase.ErrNoActiveMatch
		}

		c.deps.Games.Abandon(c.id)

	case cmdState:
		state, err := c.deps.Games.State(c.id)
		if err != nil {
			return frame{}, err
		}

		return frame{
			Type:    string(usecase.EventStateChanged),
			MatchID: string(state.MatchID),
			State:   newMatchState(state),
		}, nil

	default:
		return frame{}, errUnknownCmd
	}

	return frame{
		Type: frameAck,
		Of:   cmd.Type,
	}, nil
}
