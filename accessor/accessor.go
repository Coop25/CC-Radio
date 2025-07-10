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
    mu             sync.Mutex
    queue          []Song    // master list
    recentHistory  []string  // for queue: last ⌊len(queue)/2⌋
    randomNext     []Song    // permanent radio segment list
    randomHistory  []string  // last ⌊len(randomNext)/2⌋ segments
    lastRandom     time.Time
    cooldown       time.Duration
    maxChance      float64
    rng            *rand.Rand
    NewSongCh      chan struct{}
    forceNextRadio bool
}

func NewPlaylist(cfg *config.Config) *Playlist {
    src := rand.NewSource(time.Now().UnixNano())
    return &Playlist{
        cooldown:   cfg.RandomCooldown,
        maxChance:  cfg.RandomMaxChance,
        rng:        rand.New(src),
        NewSongCh:  make(chan struct{}, 1),
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

func (p *Playlist) AddRadio(song Song) {
    p.mu.Lock()
    wasEmpty := len(p.randomNext) == 0
    p.randomNext = append(p.randomNext, song)
    p.mu.Unlock()

    if wasEmpty {
        select { case p.NewSongCh <- struct{}{}: default: }
    }
}

func (p *Playlist) Remove(id string) {
    p.mu.Lock()
    defer p.mu.Unlock()

    // remove from queue
    q := p.queue[:0]
    for _, s := range p.queue {
        if s.ID != id {
            q = append(q, s)
        }
    }
    p.queue = q

    // remove from randomNext
    r := p.randomNext[:0]
    for _, s := range p.randomNext {
        if s.ID != id {
            r = append(r, s)
        }
    }
    p.randomNext = r

    // also clear any history entries for that ID
    p.recentHistory = filterOut(p.recentHistory, id)
    p.randomHistory = filterOut(p.randomHistory, id)
}

// Next returns either a forced‐next/radio‐segment, a randomNext bump,
// or a random master‐queue song.  Both lists cycle without repeats
// until half their items have played.
func (p *Playlist) Next() (Song, bool) {
    p.mu.Lock()
    defer p.mu.Unlock()

    now := time.Now()

    // 0) Forced radio segment
    if p.forceNextRadio && len(p.randomNext) > 0 {
        song := p.pickRandomSegment()
        p.forceNextRadio = false
        p.lastRandom = now
        return song, true
    }

    // 1) Regular randomNext bump
    if now.Sub(p.lastRandom) >= p.cooldown && len(p.randomNext) > 0 {
        song := p.pickRandomSegment()
        p.lastRandom = now
        return song, true
    }

    // 2) Master queue
    if len(p.queue) == 0 {
        return Song{}, false
    }
    song := p.pickRandomFromQueue()
    return song, true
}

// pickRandomSegment picks one from randomNext without mutating it.
func (p *Playlist) pickRandomSegment() Song {
    // build allowed pool without mutating p.randomNext
    cold := make(map[string]struct{}, len(p.randomHistory))
    for _, id := range p.randomHistory {
        cold[id] = struct{}{}
    }

    allowed := make([]Song, 0, len(p.randomNext))
    for _, s := range p.randomNext {
        if _, isCold := cold[s.ID]; !isCold {
            allowed = append(allowed, s)
        }
    }
    if len(allowed) == 0 {
        // everyone’s cold → reset history and allow all
        p.randomHistory = nil
        allowed = append(allowed, p.randomNext...)
    }

    song := allowed[p.rng.Intn(len(allowed))]
    p.recordRandomHistory(song.ID)
    return song
}

// pickRandomFromQueue picks one from queue without mutating it.
func (p *Playlist) pickRandomFromQueue() Song {
    cold := make(map[string]struct{}, len(p.recentHistory))
    for _, id := range p.recentHistory {
        cold[id] = struct{}{}
    }

    allowed := make([]Song, 0, len(p.queue))
    for _, s := range p.queue {
        if _, isCold := cold[s.ID]; !isCold {
            allowed = append(allowed, s)
        }
    }
    if len(allowed) == 0 {
        p.recentHistory = nil
        allowed = append(allowed, p.queue...)
    }

    song := allowed[p.rng.Intn(len(allowed))]
    p.recordQueueHistory(song.ID)
    return song
}

// recordQueueHistory appends id, keeping ≤ floor(len(queue)/2)
func (p *Playlist) recordQueueHistory(id string) {
    p.recentHistory = append(p.recentHistory, id)
    maxLen := len(p.queue) / 2
    if maxLen < 1 {
        maxLen = 1
    }
    if len(p.recentHistory) > maxLen {
        p.recentHistory = p.recentHistory[len(p.recentHistory)-maxLen:]
    }
}

// recordRandomHistory appends id, keeping ≤ floor(len(randomNext)/2)
func (p *Playlist) recordRandomHistory(id string) {
    p.randomHistory = append(p.randomHistory, id)
    maxLen := len(p.randomNext) / 2
    if maxLen < 1 {
        maxLen = 1
    }
    if len(p.randomHistory) > maxLen {
        p.randomHistory = p.randomHistory[len(p.randomHistory)-maxLen:]
    }
}

// ForceNextRadioSegment makes the very next Next() return from randomNext.
func (p *Playlist) ForceNextRadioSegment() {
    p.mu.Lock()
    p.forceNextRadio = true
    p.mu.Unlock()
}

// helper to filter out an ID from a string slice
func filterOut(slice []string, id string) []string {
    out := slice[:0]
    for _, x := range slice {
        if x != id {
            out = append(out, x)
        }
    }
    return out
}
