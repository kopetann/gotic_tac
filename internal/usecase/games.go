package usecase

import (
	"context"
	"log/slog"
	"sync"

	"github.com/kopetann/gotic_tac/internal/domain"
)

type Games struct {
	mu       sync.Mutex
	active   map[domain.MatchID]*activeMatch
	byPlayer map[domain.PlayerID]domain.MatchID

	matches  MatchStore
	players  PlayerRecorder
	notifier Notifier
	ids      IDGenerator
	log      *slog.Logger
}

// names is cached at start so building an event never has to hit the store.
type activeMatch struct {
	match *domain.Match
	names map[domain.PlayerID]string
}

func NewGames(matches MatchStore, players PlayerRecorder, notifier Notifier, ids IDGenerator, log *slog.Logger) *Games {
	return &Games{
		active:   make(map[domain.MatchID]*activeMatch),
		byPlayer: make(map[domain.PlayerID]domain.MatchID),
		matches:  matches,
		players:  players,
		notifier: notifier,
		ids:      ids,
		log:      log,
	}
}

func (game *Games) Start(ctx context.Context, x, o domain.PlayerID) error {
	xPlayer, err := game.players.ByID(ctx, x)
	if err != nil {
		return err
	}

	oPlayer, err := game.players.ByID(ctx, o)
	if err != nil {
		return err
	}

	match, err := domain.NewMatch(domain.MatchID(game.ids.NewID()), x, o)
	if err != nil {
		return err
	}

	activeMatch := &activeMatch{
		match: match,
		names: map[domain.PlayerID]string{
			x: xPlayer.Name,
			o: oPlayer.Name,
		},
	}

	game.mu.Lock()
	defer game.mu.Unlock()

	if _, busy := game.byPlayer[x]; busy {
		return ErrAlreadyInMatch
	}

	if _, busy := game.byPlayer[o]; busy {
		return ErrAlreadyInMatch
	}

	game.active[match.ID()] = activeMatch

	game.byPlayer[x] = match.ID()
	game.byPlayer[o] = match.ID()

	rec, evs := activeMatch.record(), activeMatch.events(EventMatchStarted)

	game.persist(ctx, rec)
	game.dispatch(evs)
	return nil
}

func (g *Games) Move(ctx context.Context, p domain.PlayerID, cell int) error {
	g.mu.Lock()
	am, ok := g.lookup(p)
	if !ok {
		g.mu.Unlock()
		return ErrNoActiveMatch
	}

	// The domain error is returned unchanged: the transport maps its Code, so
	// wrapping or flattening it here would break the protocol.
	if err := am.match.ApplyMove(p, cell); err != nil {
		g.mu.Unlock()
		return err
	}

	evType := EventStateChanged
	var results []result
	if am.match.Status().IsFinished() {
		evType = EventMatchEnded
		results = am.results()
		g.forget(am)
	}

	rec, evs := am.record(), am.events(evType)
	g.mu.Unlock()

	g.persist(ctx, rec)
	for _, r := range results {
		g.record(ctx, r)
	}

	g.dispatch(evs)
	return nil
}

func (g *Games) State(p domain.PlayerID) (MatchState, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	am, ok := g.lookup(p)
	if !ok {
		return MatchState{}, ErrNoActiveMatch
	}

	return am.stateFor(p), nil
}

func (g *Games) InMatch(p domain.PlayerID) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	_, ok := g.byPlayer[p]
	return ok
}

// Abandon drops the match
func (g *Games) Abandon(p domain.PlayerID) {
	g.mu.Lock()
	am, ok := g.lookup(p)
	if !ok {
		g.mu.Unlock()
		return
	}

	opponent := am.match.Opponent(p)
	ev := Event{Type: EventOpponentLeft, MatchID: am.match.ID()}
	g.forget(am)
	g.mu.Unlock()

	g.notifier.Notify(opponent, ev)
}

func (g *Games) lookup(p domain.PlayerID) (*activeMatch, bool) {
	id, ok := g.byPlayer[p]
	if !ok {
		return nil, false
	}

	am, ok := g.active[id]
	return am, ok
}

func (g *Games) forget(am *activeMatch) {
	delete(g.active, am.match.ID())
	delete(g.byPlayer, am.match.PlayerOf(domain.X))
	delete(g.byPlayer, am.match.PlayerOf(domain.O))
}

// stateFor is per-player because YourMark and Outcome differ by recipient.
// That is why Notifier addresses one player rather than broadcasting.
func (am *activeMatch) stateFor(p domain.PlayerID) MatchState {
	mark, _ := am.match.MarkOf(p)

	return MatchState{
		MatchID:  am.match.ID(),
		Cells:    am.match.Board(),
		YourMark: mark,
		Turn:     am.match.Turn(),
		Status:   am.match.Status(),
		Outcome:  am.match.OutcomeFor(p),
		Opponent: am.names[am.match.Opponent(p)],
	}
}

func (am *activeMatch) events(t EventType) []addressed {
	x, o := am.match.PlayerOf(domain.X), am.match.PlayerOf(domain.O)

	return []addressed{
		{to: x, ev: Event{Type: t, MatchID: am.match.ID(), State: am.stateFor(x)}},
		{to: o, ev: Event{Type: t, MatchID: am.match.ID(), State: am.stateFor(o)}},
	}
}

func (am *activeMatch) record() MatchRecord {
	return MatchRecord{
		ID:     am.match.ID(),
		X:      am.match.PlayerOf(domain.X),
		O:      am.match.PlayerOf(domain.O),
		Status: am.match.Status(),
		Moves:  am.match.Moves(),
	}
}

func (am *activeMatch) results() []result {
	x, o := am.match.PlayerOf(domain.X), am.match.PlayerOf(domain.O)

	return []result{
		{player: x, outcome: am.match.OutcomeFor(x)},
		{player: o, outcome: am.match.OutcomeFor(o)},
	}
}

type addressed struct {
	to domain.PlayerID
	ev Event
}

type result struct {
	player  domain.PlayerID
	outcome domain.Outcome
}

func (g *Games) dispatch(evs []addressed) {
	for _, e := range evs {
		g.notifier.Notify(e.to, e.ev)
	}
}

func (g *Games) persist(ctx context.Context, rec MatchRecord) {
	if err := g.matches.Save(ctx, rec); err != nil {
		g.log.Error("save match", "match_id", rec.ID, "error", err)
	}
}

func (g *Games) record(ctx context.Context, r result) {
	p, err := g.players.ByID(ctx, r.player)
	if err != nil {
		g.log.Error("load player for stats", "player_id", r.player, "error", err)
		return
	}

	p.Record(r.outcome)
	if err := g.players.Save(ctx, p); err != nil {
		g.log.Error("save player stats", "player_id", r.player, "error", err)
	}
}
