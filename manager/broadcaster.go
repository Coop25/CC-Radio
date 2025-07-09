package manager

import (
	"context"
	"sync"
	"time"

	"github.com/Coop25/CC-Radio/accessor"
	"github.com/Coop25/CC-Radio/chunker"
	"github.com/gorilla/websocket"
)

type Broadcaster struct {
	conns    map[*websocket.Conn]struct{}
	mu       sync.Mutex
	interval time.Duration
	playlist *accessor.Playlist
	cancel   context.CancelFunc
	fetcher  accessor.Fetcher
}

// NewBroadcaster starts the ticker loop; you can call Start(ctx) to begin.
func NewBroadcaster(interval time.Duration, pl *accessor.Playlist, f accessor.Fetcher) *Broadcaster {
	return &Broadcaster{
		conns:    make(map[*websocket.Conn]struct{}),
		interval: interval,
		playlist: pl,
		fetcher:  f,
	}
}

func (b *Broadcaster) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	b.cancel = cancel

	go func() {
		ticker := time.NewTicker(b.interval)
		defer ticker.Stop()

		// Load initial track (if any):
		current, ok := b.playlist.Next()
		if !ok {
			<-ctx.Done()
			return // nothing to do
		}
		next, _ := b.playlist.Next()
		currSlices := b.loadSlices(current)
		nextSlices := b.loadSlices(next)
		idx := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.mu.Lock()
				for c := range b.conns {
					c.WriteMessage(websocket.BinaryMessage, currSlices[idx])
				}
				b.mu.Unlock()
				idx++
				if idx >= len(currSlices) {
					// rotate tracks
					currSlices = nextSlices
					current = next
					// queue up a new “next”
					next, _ = b.playlist.Next()
					nextSlices = b.loadSlices(next)
					idx = 0
				}
			}
		}
	}()
}

func (b *Broadcaster) loadSlices(s accessor.Song) [][]byte {
	data, err := b.fetcher.FetchBytes(s.ID)
	if err != nil {
		// handle/log error and maybe return empty slice or panic, as appropriate
		panic(err)
	}
	return chunker.PrepareChunks(data, s.Duration, b.interval)
}

func (b *Broadcaster) Skip() {
	if b.cancel != nil {
		b.cancel() // stop current loop
		// restart a fresh broadcaster using new context
		// …similar logic to Start…
	}
}

func (b *Broadcaster) Register(conn *websocket.Conn) {
	b.mu.Lock()
	b.conns[conn] = struct{}{}
	b.mu.Unlock()
}
func (b *Broadcaster) Unregister(conn *websocket.Conn) {
	b.mu.Lock()
	delete(b.conns, conn)
	b.mu.Unlock()
}
