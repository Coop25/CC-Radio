package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Coop25/CC-Radio/accessor"
	"github.com/Coop25/CC-Radio/chunker"
	"github.com/Coop25/CC-Radio/config"
	"github.com/gorilla/websocket"
)

type songChangeMsg struct {
	Type     string        `json:"type"`
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Artist   string        `json:"artist"`
	Duration time.Duration `json:"duration"`
}

type Broadcaster struct {
	conns       map[*websocket.Conn]struct{}
	mu          sync.Mutex
	interval    time.Duration
	skipCh      chan struct{}
	playlist    *accessor.Playlist
	cancel      context.CancelFunc
	fetcher     accessor.Fetcher
	webhook     string
	http        *http.Client
	currentSong accessor.Song // ← track what’s playing
}

// NewBroadcaster starts the ticker loop; you can call Start(ctx) to begin.
func NewBroadcaster(cfg *config.Config, pl *accessor.Playlist, f accessor.Fetcher) *Broadcaster {
	return &Broadcaster{
		conns:    make(map[*websocket.Conn]struct{}),
		interval: cfg.ChunkInterval,
		skipCh:   make(chan struct{}, 1),
		playlist: pl,
		fetcher:  f,
		webhook:  cfg.NowPlayingWebhookURL,
		http:     &http.Client{Timeout: 5 * time.Second},
	}
}

// notifySongChange sends a JSON text frame to all clients indicating the new track.
func (b *Broadcaster) notifySongChange(song accessor.Song) {
	msg := songChangeMsg{
		Type:     "songChange",
		ID:       song.ID,
		Name:     song.Name,
		Artist:   song.Artist,
		Duration: song.Duration,
	}
	payload, _ := json.Marshal(msg)

	b.mu.Lock()
	defer b.mu.Unlock()
	for conn := range b.conns {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			log.Printf("[Broadcaster] songChange WriteMessage error: %v", err)
		}
	}
}

func (b *Broadcaster) announce(song accessor.Song) {
	if b.webhook == "" {
		return
	}
	payload := map[string]string{
		"content": "🎶 Now playing: **" + song.Name + "** by *" + song.Artist + "*",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", b.webhook, bytes.NewReader(body))
	if err != nil {
		log.Printf("[Webhook] NewRequest error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.http.Do(req)
	if err != nil {
		log.Printf("[Webhook] POST error: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("[Webhook] non-2xx status: %s", resp.Status)
	}
}

// prefetchSlices keeps retrying FetchBytes(song.ID) until it succeeds,
// then sends the prepared chunks on out.
func (b *Broadcaster) prefetchSlices(song accessor.Song, out chan<- [][]byte) {
	go func() {
		for {
			data, err := b.fetcher.FetchBytes(song.ID)
			if err != nil {
				log.Printf("[Broadcaster] Prefetch %s error: %v; retrying in 5s", song.ID, err)
				time.Sleep(5 * time.Second)
				continue
			}
			chunks := chunker.PrepareChunks(data, song.Duration, b.interval)
			out <- chunks
			return
		}
	}()
}

func (b *Broadcaster) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	b.cancel = cancel

	go func() {
		log.Printf("[Broadcaster] Starting with interval %v", b.interval)
		ticker := time.NewTicker(b.interval)
		defer ticker.Stop()

		var (
			currSlices [][]byte
			nextSlices [][]byte
			idx        int
			current    accessor.Song
			next       accessor.Song
		)

		// Phase 0: wait for first song
		log.Printf("[Broadcaster] Waiting for first song…")
		for {
			track, ok := b.playlist.Next()
			if !ok {
				log.Printf("[Broadcaster] No song yet, blocking until NewSongCh")
				select {
				case <-b.playlist.NewSongCh:
					continue
				case <-ctx.Done():
					return
				}
			}
			current = track
			b.currentSong = current
			log.Printf("[Broadcaster] Loaded initial track: ID=%s, Duration=%v", current.ID, current.Duration)
			break
		}
		b.notifySongChange(current)
		b.announce(current)

		// Pick and prefetch next
		if nt, ok := b.playlist.Next(); ok {
			next = nt
			log.Printf("[Broadcaster] Preloading next track: ID=%s", next.ID)
		}
		currSlices = b.loadSlices(current)

		prefetchCh := make(chan [][]byte, 1)
		b.prefetchSlices(next, prefetchCh)
		nextSlices = <-prefetchCh
		log.Printf("[Broadcaster] Prepared %d chunks for current, %d for next", len(currSlices), len(nextSlices))
		idx = 0

		// Phase 1: main loop
		for {
			select {
			case <-ctx.Done():
				log.Printf("[Broadcaster] Stopping")
				return

			case <-ticker.C:
				if len(currSlices) == 0 {
					continue
				}
				b.mu.Lock()
				for conn := range b.conns {
					if err := conn.WriteMessage(websocket.BinaryMessage, currSlices[idx]); err != nil {
						log.Printf("[Broadcaster] WriteMessage error: %v", err)
					}
				}
				b.mu.Unlock()

				idx++
				if idx >= len(currSlices) {
					// rotate
					log.Printf("[Broadcaster] Finished %s; rotating to %s", current.ID, next.ID)
					currSlices = nextSlices
					current = next
					b.currentSong = current

					b.notifySongChange(current)
					b.announce(current)

					// fetch the following track in background
					if nt, ok := b.playlist.Next(); ok {
						next = nt
						log.Printf("[Broadcaster] Preloading next track: ID=%s", next.ID)
					}
					prefetchCh = make(chan [][]byte, 1)
					b.prefetchSlices(next, prefetchCh)
					nextSlices = <-prefetchCh
					log.Printf("[Broadcaster] New next: %s (%d chunks)", next.ID, len(nextSlices))

					idx = 0
				}

			case <-b.skipCh:
				log.Printf("[Broadcaster] Skip received; rotating immediately")
				currSlices = nextSlices
				current = next
				b.currentSong = current

				b.notifySongChange(current)
				b.announce(current)

				if nt, ok := b.playlist.Next(); ok {
					next = nt
					log.Printf("[Broadcaster] Preloading next track: ID=%s", next.ID)
				}
				prefetchCh = make(chan [][]byte, 1)
				b.prefetchSlices(next, prefetchCh)
				nextSlices = <-prefetchCh
				log.Printf("[Broadcaster] Now playing %s; next queued %s", current.ID, next.ID)
				idx = 0

			case <-b.playlist.NewSongCh:
				if len(nextSlices) == 0 {
					log.Printf("[Broadcaster] New song mid‐playback; loading as next")
					if nt, ok := b.playlist.Next(); ok {
						next = nt
						log.Printf("[Broadcaster] Preloading next track: ID=%s", next.ID)
					}
					prefetchCh = make(chan [][]byte, 1)
					b.prefetchSlices(next, prefetchCh)
					nextSlices = <-prefetchCh
					log.Printf("[Broadcaster] Loaded next: %s (%d chunks)", next.ID, len(nextSlices))
				} else {
					log.Printf("[Broadcaster] Detected new song mid‐playback; will queue after current finishes")
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

// Skip signals an immediate jump to the pre‐queued track.
func (b *Broadcaster) Skip() {
	select {
	case b.skipCh <- struct{}{}:
	default:
	}
}

func (b *Broadcaster) DeleteCurrent() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.currentSong.ID == "" {
		return fmt.Errorf("no current song to delete")
	}
	id := b.currentSong.ID
	b.playlist.Remove(id)
	log.Printf("[Broadcaster] Deleted current song %s from queue & randomNext", id)
	b.Skip()
	return nil
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
