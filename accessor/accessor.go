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
    mu            sync.Mutex
    // ────────────────────────────────────────────────────────
    // Master queue fields
    queue         []Song    // master list
    recentHistory []string  // last 2 IDs played
    shuffledQueue []Song    // clone of queue
    shuffledIndex int       // next index into shuffledQueue

    // Random-next (radio segment) fields
    randomNext         []Song
    randomHistory      []string  // last 2 segment IDs played
    shuffledRadio      []Song    // clone of randomNext
    shuffledRadioIndex int       // next index into shuffledRadio

    // shared RNG & timing
    rng           *rand.Rand
    cooldown      time.Duration
    maxChance     float64
    lastRandom    time.Time
    NewSongCh     chan struct{}
    forceNextRadio bool
}

func NewPlaylist(cfg *config.Config) *Playlist {
    src := rand.NewSource(time.Now().UnixNano())
    return &Playlist{
        rng:            rand.New(src),
        cooldown:       cfg.RandomCooldown,
        maxChance:      cfg.RandomMaxChance,
        NewSongCh:      make(chan struct{}, 1),
        recentHistory:  nil,
        randomHistory:  nil,
    }
}

func (p *Playlist) Add(song Song) {
    p.mu.Lock()
    exists := false
    for _, s := range p.queue {
        if s.ID == song.ID {
            exists = true
            break
        }
    }
    if !exists {
        wasEmpty := len(p.queue) == 0
        p.queue = append(p.queue, song)

        // also tack onto the end of the current shuffle buffer:
        p.shuffledQueue = append(p.shuffledQueue, song)

        // signal “first song” if this was the very first
        if wasEmpty {
            select { case p.NewSongCh <- struct{}{}: default: }
        }
    }
    p.mu.Unlock()
}

func (p *Playlist) AddRadio(song Song) {
    p.mu.Lock()
    exists := false
    for _, s := range p.randomNext {
        if s.ID == song.ID {
            exists = true
            break
        }
    }
    if !exists {
        wasEmpty := len(p.randomNext) == 0
        p.randomNext = append(p.randomNext, song)

        // same trick for your radio‐segment shuffle buffer
        p.shuffledRadio = append(p.shuffledRadio, song)

        if wasEmpty {
            select { case p.NewSongCh <- struct{}{}: default: }
        }
    }
    p.mu.Unlock()
}

func (p *Playlist) Remove(id string) {
    p.mu.Lock()
    defer p.mu.Unlock()
    // queue
    q := p.queue[:0]
    for _, s := range p.queue {
        if s.ID != id { q = append(q, s) }
    }
    p.queue = q
    // radio
    r := p.randomNext[:0]
    for _, s := range p.randomNext {
        if s.ID != id { r = append(r, s) }
    }
    p.randomNext = r
    // histories
    p.recentHistory = filterOut(p.recentHistory, id)
    p.randomHistory = filterOut(p.randomHistory, id)
}

// Next returns forced radio, radio bump, or master queue track.
func (p *Playlist) Next() (Song, bool) {
    p.mu.Lock()
    defer p.mu.Unlock()

    now := time.Now()
    // 0) forced radio segment
    if p.forceNextRadio && len(p.randomNext) > 0 {
        s, _ := p.popShuffledRadio()
        p.forceNextRadio = false
        p.lastRandom = now
        return s, true
    }
    // 1) regular radio bump
    if now.Sub(p.lastRandom) >= p.cooldown && len(p.randomNext) > 0 {
        s, _ := p.popShuffledRadio()
        p.lastRandom = now
        return s, true
    }
    // 2) master queue
    if len(p.queue) == 0 {
        return Song{}, false
    }
    s, ok := p.popShuffled()
    return s, ok
}

// popShuffledRadio pops from shuffledRadio, refilling if needed.
func (p *Playlist) popShuffledRadio() (Song, bool) {
    if p.shuffledRadio == nil || p.shuffledRadioIndex >= len(p.shuffledRadio) {
        p.refillShuffledRadio()
    }
    if len(p.shuffledRadio) == 0 {
        return Song{}, false
    }
    s := p.shuffledRadio[p.shuffledRadioIndex]
    p.shuffledRadioIndex++
    p.recordRandomHistory(s.ID)
    return s, true
}

// refillShuffledRadio clones & shuffles randomNext, moves last 2 into middle.
func (p *Playlist) refillShuffledRadio() {
    n := len(p.randomNext)
    p.shuffledRadio = append([]Song(nil), p.randomNext...)
    p.rng.Shuffle(n, func(i, j int) {
        p.shuffledRadio[i], p.shuffledRadio[j] = p.shuffledRadio[j], p.shuffledRadio[i]
    })
    mid := n/2
    // bias last two into center
    for i, id := range p.randomHistory {
        for j, s := range p.shuffledRadio {
            if s.ID == id {
                target := mid + (i - len(p.randomHistory)/2)
                if target<0 { target=0 }
                if target>=n { target=n-1 }
                p.shuffledRadio[j], p.shuffledRadio[target] = p.shuffledRadio[target], p.shuffledRadio[j]
                break
            }
        }
    }
    p.shuffledRadioIndex = 0
}

// popShuffled pops from shuffledQueue, refilling if needed.
func (p *Playlist) popShuffled() (Song, bool) {
    if p.shuffledQueue == nil || p.shuffledIndex >= len(p.shuffledQueue) {
        p.refillShuffled()
    }
    if len(p.shuffledQueue) == 0 {
        return Song{}, false
    }
    s := p.shuffledQueue[p.shuffledIndex]
    p.shuffledIndex++
    p.recordQueueHistory(s.ID)
    return s, true
}

// refillShuffled clones & shuffles queue, moves last 2 into middle.
func (p *Playlist) refillShuffled() {
    n := len(p.queue)
    p.shuffledQueue = append([]Song(nil), p.queue...)
    p.rng.Shuffle(n, func(i, j int) {
        p.shuffledQueue[i], p.shuffledQueue[j] = p.shuffledQueue[j], p.shuffledQueue[i]
    })
    mid := n/2
    for i, id := range p.recentHistory {
        for j, s := range p.shuffledQueue {
            if s.ID == id {
                target := mid + (i - len(p.recentHistory)/2)
                if target<0 { target=0 }
                if target>=n { target=n-1 }
                p.shuffledQueue[j], p.shuffledQueue[target] = p.shuffledQueue[target], p.shuffledQueue[j]
                break
            }
        }
    }
    p.shuffledIndex = 0
}

func (p *Playlist) recordQueueHistory(id string) {
    p.recentHistory = append(p.recentHistory, id)
    if len(p.recentHistory) > 2 {
        p.recentHistory = p.recentHistory[len(p.recentHistory)-2:]
    }
}

func (p *Playlist) recordRandomHistory(id string) {
    p.randomHistory = append(p.randomHistory, id)
    if len(p.randomHistory) > 2 {
        p.randomHistory = p.randomHistory[len(p.randomHistory)-2:]
    }
}

func (p *Playlist) ForceNextRadioSegment() {
    p.mu.Lock()
    p.forceNextRadio = true
    p.mu.Unlock()
}

// filterOut removes id from a string slice.
func filterOut(slice []string, id string) []string {
    out := slice[:0]
    for _, x := range slice {
        if x != id {
            out = append(out, x)
        }
    }
    return out
}
