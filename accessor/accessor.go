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
    mu           sync.Mutex
    queue        []Song
    currentIndex int       // index into queue
    randomNext   []Song
    randomIndex  int       // index into randomNext
    lastRandom   time.Time
    cooldown     time.Duration
    maxChance    float64
    rng          *rand.Rand
    NewSongCh    chan struct{}
}

func NewPlaylist(cooldown time.Duration, maxChance float64) *Playlist {
    src := rand.NewSource(time.Now().UnixNano())
    return &Playlist{
        cooldown:     cooldown,
        maxChance:    maxChance,
        rng:          rand.New(src),
        NewSongCh:    make(chan struct{}, 1),
        currentIndex: 0,
        randomIndex:  0,
    }
}

func (p *Playlist) Add(song Song) {
    p.mu.Lock()
    wasEmpty := len(p.queue) == 0
    p.queue = append(p.queue, song)
    p.mu.Unlock()

    if wasEmpty {
        select { case p.NewSongCh <- struct{}{}: default: }
    }
}

func (p *Playlist) Shuffle() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.rng.Shuffle(len(p.queue), func(i, j int) {
        p.queue[i], p.queue[j] = p.queue[j], p.queue[i]
    })
    p.currentIndex = 0
}

// Next returns either a randomNext bump (cycling through that slice),
// or the next song in the main queue (cycling as well).  Neither slice
// is ever mutated—indices simply wrap around.
func (p *Playlist) Next() (song Song, ok bool) {
    p.mu.Lock()
    defer p.mu.Unlock()

    now := time.Now()
    // 1) randomNext bump (unchanged)
    if now.Sub(p.lastRandom) >= p.cooldown && len(p.randomNext) > 0 {
        chance := (now.Sub(p.lastRandom).Seconds() / p.cooldown.Seconds()) * p.maxChance
        if p.rng.Float64() < chance {
            song = p.randomNext[p.randomIndex]
            p.randomIndex = (p.randomIndex + 1) % len(p.randomNext)
            p.lastRandom = now
            return song, true
        }
    }

    // 2) main queue cycling + shuffle on wrap
    n := len(p.queue)
    if n == 0 {
        return Song{}, false
    }

    // pick the current song
    song = p.queue[p.currentIndex]

    // advance index, and if we hit the end, reshuffle & reset
    p.currentIndex++
    if p.currentIndex >= n {
        // shuffle for the next cycle
        p.rng.Shuffle(n, func(i, j int) {
            p.queue[i], p.queue[j] = p.queue[j], p.queue[i]
        })
        p.currentIndex = 0
    }

    return song, true
}