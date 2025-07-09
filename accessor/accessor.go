package accessor

import (
    "math/rand"
    "sync"
    "time"
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
    queue      []Song
    randomNext []Song
    lastRandom time.Time
    cooldown   time.Duration
    maxChance  float64
    rng        *rand.Rand
	NewSongCh chan struct{}
}

// NewPlaylist creates a Playlist with its own RNG seeded from the current time.
func NewPlaylist(cooldown time.Duration, maxChance float64) *Playlist {
    // Create a local random generator rather than seeding the global one.
    src := rand.NewSource(time.Now().UnixNano())
    return &Playlist{
        cooldown:  cooldown,
        maxChance: maxChance,
        rng:       rand.New(src),
        NewSongCh: make(chan struct{}, 1),
    }
}

func (p *Playlist) Add(song Song) {
    p.mu.Lock()
    emptyBefore := len(p.queue) == 0
    p.queue = append(p.queue, song)
    p.mu.Unlock()

    // if we just went from 0→1, notify the broadcaster
    if emptyBefore {
        select {
        case p.NewSongCh <- struct{}{}:
        default:
        }
    }
}

func (p *Playlist) Shuffle() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.rng.Shuffle(len(p.queue), func(i, j int) {
        p.queue[i], p.queue[j] = p.queue[j], p.queue[i]
    })
}

// Next returns either a “randomNext” track (respecting cooldown + chance)
// or the next song in the master queue (reshuffling when it empties).
// If there’s nothing to play, ok=false.
func (p *Playlist) Next() (song Song, ok bool) {
    p.mu.Lock()
    defer p.mu.Unlock()

    now := time.Now()

    // 1) Maybe play a randomNext track
    if now.Sub(p.lastRandom) >= p.cooldown && len(p.randomNext) > 0 {
        // chance grows over time up to maxChance
        chance := (now.Sub(p.lastRandom).Seconds() / p.cooldown.Seconds()) * p.maxChance
        if p.rng.Float64() < chance {
            idx := p.rng.Intn(len(p.randomNext))
            song = p.randomNext[idx]
            p.randomNext = append(p.randomNext[:idx], p.randomNext[idx+1:]...)
            p.lastRandom = now
            return song, true
        }
    }

    // 2) Fall back to the master queue
    if len(p.queue) == 0 {
        return Song{}, false
    }
    song = p.queue[0]
    p.queue = p.queue[1:]
    if len(p.queue) == 0 {
        // once drained, reshuffle for the next cycle
        p.rng.Shuffle(len(p.queue), func(i, j int) {
            p.queue[i], p.queue[j] = p.queue[j], p.queue[i]
        })
    }
    return song, true
}
