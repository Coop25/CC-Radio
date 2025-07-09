package accessor

import (
	"math/rand"
	"sync"
	"time"

	"github.com/Coop25/CC-Radio/config"
)

type Song struct {
	ID       string
	Name     string
	Artist   string
	URL      string
	Duration time.Duration
}

type Playlist struct {
	mu         sync.Mutex
	queue      []Song // full master list
	remaining  []Song // songs left in this random cycle
	randomNext []Song
	lastRandom time.Time
	cooldown   time.Duration
	maxChance  float64
	rng        *rand.Rand
	NewSongCh  chan struct{}
	lastSongID string
}

func NewPlaylist(cfg *config.Config) *Playlist {
	src := rand.NewSource(time.Now().UnixNano())
	return &Playlist{
		queue:      make([]Song, 0),
		remaining:  nil,
		randomNext: make([]Song, 0),
		cooldown:   cfg.RandomCooldown,
		maxChance:  cfg.RandomMaxChance,
		rng:        rand.New(src),
		NewSongCh:  make(chan struct{}, 1),
	}
}

// Remove deletes every occurrence of the given song ID from both
// the main queue and the randomNext slice.
func (p *Playlist) Remove(id string) {
    p.mu.Lock()
    defer p.mu.Unlock()

    // Filter main queue
    newQ := p.queue[:0]
    for _, s := range p.queue {
        if s.ID != id {
            newQ = append(newQ, s)
        }
    }
    p.queue = newQ

    // Filter randomNext
    newR := p.randomNext[:0]
    for _, s := range p.randomNext {
        if s.ID != id {
            newR = append(newR, s)
        }
    }
    p.randomNext = newR
}


func (p *Playlist) Add(song Song) {
	p.mu.Lock()
	wasEmpty := len(p.queue) == 0
	// append to master list
	p.queue = append(p.queue, song)
	// also make it available in the current cycle
	p.remaining = append(p.remaining, song)
	p.mu.Unlock()

	if wasEmpty {
		select {
		case p.NewSongCh <- struct{}{}:
		default:
		}
	}
}

func (p *Playlist) Shuffle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// reset remaining to a fresh copy of queue
	p.remaining = append([]Song(nil), p.queue...)
	// shuffle that slice
	p.rng.Shuffle(len(p.remaining), func(i, j int) {
		p.remaining[i], p.remaining[j] = p.remaining[j], p.remaining[i]
	})
	// clear lastSongID so the first pick won't be blocked
	p.lastSongID = ""
}

// Next returns either a randomNext bump (unchanged) or a truly random
// song from the main queue, never repeating the same ID twice in a row.
func (p *Playlist) Next() (Song, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	// 1) randomNext bump
	if now.Sub(p.lastRandom) >= p.cooldown && len(p.randomNext) > 0 {
		idx := p.rng.Intn(len(p.randomNext))
		song := p.randomNext[idx]
		// drop it from randomNext
		p.randomNext = append(p.randomNext[:idx], p.randomNext[idx+1:]...)
		p.lastRandom = now
		if song.ID != p.lastSongID {
			p.lastSongID = song.ID
			return song, true
		}
		// if it was the same, we fall back to the main queue
	}

	// 2) main queue random cycle
	if len(p.remaining) == 0 {
		// refill and shuffle for a new cycle
		p.remaining = append([]Song(nil), p.queue...)
		p.rng.Shuffle(len(p.remaining), func(i, j int) {
			p.remaining[i], p.remaining[j] = p.remaining[j], p.remaining[i]
		})
	}

	if len(p.remaining) == 0 {
		return Song{}, false
	}

	// pick a random index in remaining
	idx := p.rng.Intn(len(p.remaining))
	song := p.remaining[idx]

	// if it would repeat, and there’s >1 left, pick the next different one
	if song.ID == p.lastSongID && len(p.remaining) > 1 {
		// swap it with the next slot (mod len)
		idx2 := (idx + 1) % len(p.remaining)
		song = p.remaining[idx2]
		idx = idx2
	}

	// remove from remaining
	p.remaining = append(p.remaining[:idx], p.remaining[idx+1:]...)
	p.lastSongID = song.ID
	return song, true
}
