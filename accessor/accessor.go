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
    mu              sync.Mutex
    queue           []Song           // master list
    randomNext      []Song           // radio segments
    lastPlayed      map[string]time.Time
    rng             *rand.Rand
    cooldown        time.Duration
    maxChance       float64
    lastRandom      time.Time
    NewSongCh       chan struct{}
    forceNextRadio  bool
}

func NewPlaylist(cfg *config.Config) *Playlist {
    src := rand.NewSource(time.Now().UnixNano())
    return &Playlist{
        queue:          nil,
        randomNext:     nil,
        lastPlayed:     make(map[string]time.Time),
        rng:            rand.New(src),
        cooldown:       cfg.RandomCooldown,
        maxChance:      cfg.RandomMaxChance,
        NewSongCh:      make(chan struct{}, 1),
    }
}

func (p *Playlist) Add(song Song) {
    p.mu.Lock()
    wasEmpty := len(p.queue) == 0
    // only add if not already present
    for _, s := range p.queue {
        if s.ID == song.ID {
            p.mu.Unlock()
            return
        }
    }
    p.queue = append(p.queue, song)
    p.mu.Unlock()

    if wasEmpty {
        select { case p.NewSongCh <- struct{}{}: default: }
    }
}

func (p *Playlist) AddRadio(song Song) {
    p.mu.Lock()
    wasEmpty := len(p.randomNext) == 0
    // only add if not already present
    for _, s := range p.randomNext {
        if s.ID == song.ID {
            p.mu.Unlock()
            return
        }
    }
    p.randomNext = append(p.randomNext, song)
    p.mu.Unlock()

    if wasEmpty {
        select { case p.NewSongCh <- struct{}{}: default: }
    }
}

func (p *Playlist) Remove(id string) {
    p.mu.Lock()
    defer p.mu.Unlock()

    // master queue
    q := p.queue[:0]
    for _, s := range p.queue {
        if s.ID != id {
            q = append(q, s)
        }
    }
    p.queue = q

    // radio segments
    r := p.randomNext[:0]
    for _, s := range p.randomNext {
        if s.ID != id {
            r = append(r, s)
        }
    }
    p.randomNext = r

    // purge lastPlayed entry
    delete(p.lastPlayed, id)
}

// Next returns either a forced‐next/radio segment, a cooldown‐based bump,
// or a weighted‐random master‐queue track.
func (p *Playlist) Next() (Song, bool) {
    p.mu.Lock()
    defer p.mu.Unlock()

    now := time.Now()

    // 0) forced radio override
    if p.forceNextRadio && len(p.randomNext) > 0 {
        song := p.pickRandomSegment()
        p.forceNextRadio = false
        p.lastRandom = now
        return song, true
    }
    // 1) regular radio bump
    if now.Sub(p.lastRandom) >= p.cooldown && len(p.randomNext) > 0 {
        song := p.pickRandomSegment()
        p.lastRandom = now
        return song, true
    }
    // 2) weighted random master queue
    if len(p.queue) == 0 {
        return Song{}, false
    }
    song := p.weightedRandomFromQueue(now)
    // record play time
    p.lastPlayed[song.ID] = now
    return song, true
}

// weightedRandomFromQueue does a roulette‐wheel pick with weights = time since last play.
func (p *Playlist) weightedRandomFromQueue(now time.Time) Song {
    type candidate struct {
        song   Song
        weight float64
    }
    total := 0.0
    cands := make([]candidate, 0, len(p.queue))
    for _, s := range p.queue {
        last, seen := p.lastPlayed[s.ID]
        var w float64
        if !seen {
            w = 1.0  // unseen tracks get base weight
        } else {
            w = now.Sub(last).Seconds()
            if w < 0 {
                w = 0
            }
        }
        cands = append(cands, candidate{s, w})
        total += w
    }
    // if all weights zero, fallback to uniform
    if total == 0 {
        idx := p.rng.Intn(len(cands))
        return cands[idx].song
    }
    // pick a random threshold
    r := p.rng.Float64() * total
    for _, c := range cands {
        r -= c.weight
        if r <= 0 {
            return c.song
        }
    }
    // rounding fallback
    return cands[len(cands)-1].song
}

// pickRandomSegment remains unchanged
func (p *Playlist) pickRandomSegment() Song {
    // simple uniform random from p.randomNext
    idx := p.rng.Intn(len(p.randomNext))
    return p.randomNext[idx]
}

// ForceNextRadioSegment causes the very next Next() to pick from randomNext.
func (p *Playlist) ForceNextRadioSegment() {
    p.mu.Lock()
    p.forceNextRadio = true
    p.mu.Unlock()
}
