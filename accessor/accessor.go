package accessor

import (
    "math/rand"
    "sort"
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
    mu                sync.Mutex
    queue             []Song            // master list
    randomNext        []Song            // radio‐segment list
    lastPlayed        map[string]time.Time
    lastRadioPlayed   map[string]time.Time
    rng               *rand.Rand
    cooldown          time.Duration
    maxChance         float64
    lastRandom        time.Time
    NewSongCh         chan struct{}
    forceNextRadio    bool

    // these hold the current “deck” for each list
    shuffledQueue     []Song
    shuffledIndex     int
    shuffledRadio     []Song
    shuffledRadioIndex int
}

func NewPlaylist(cfg *config.Config) *Playlist {
    src := rand.NewSource(time.Now().UnixNano())
    return &Playlist{
        lastPlayed:      make(map[string]time.Time),
        lastRadioPlayed: make(map[string]time.Time),
        rng:             rand.New(src),
        cooldown:        cfg.RandomCooldown,
        maxChance:       cfg.RandomMaxChance,
        NewSongCh:       make(chan struct{}, 1),
    }
}

func (p *Playlist) Add(song Song) {
    p.mu.Lock()
    defer p.mu.Unlock()
    // de-dupe
    for _, s := range p.queue {
        if s.ID == song.ID {
            return
        }
    }
    first := len(p.queue) == 0
    p.queue = append(p.queue, song)
    if first {
        select { case p.NewSongCh <- struct{}{}: default: }
    }
}

func (p *Playlist) AddRadio(song Song) {
    p.mu.Lock()
    defer p.mu.Unlock()
    for _, s := range p.randomNext {
        if s.ID == song.ID {
            return
        }
    }
    first := len(p.randomNext) == 0
    p.randomNext = append(p.randomNext, song)
    if first {
        select { case p.NewSongCh <- struct{}{}: default: }
    }
}

func (p *Playlist) Remove(id string) {
    p.mu.Lock()
    defer p.mu.Unlock()
    // queue
    q := p.queue[:0]
    for _, s := range p.queue {
        if s.ID != id {
            q = append(q, s)
        }
    }
    p.queue = q
    delete(p.lastPlayed, id)

    // radio
    r := p.randomNext[:0]
    for _, s := range p.randomNext {
        if s.ID != id {
            r = append(r, s)
        }
    }
    p.randomNext = r
    delete(p.lastRadioPlayed, id)
}

// Next gives you the next track: forced radio, cooldown bump, or weighted master.
func (p *Playlist) Next() (Song, bool) {
    p.mu.Lock()
    defer p.mu.Unlock()

    now := time.Now()

    // 0) forced radio segment
    if p.forceNextRadio && len(p.randomNext) > 0 {
        song, _ := p.popShuffledRadio(now)
        p.forceNextRadio = false
        p.lastRandom = now
        return song, true
    }
    // 1) cooldown‐based radio bump
    if now.Sub(p.lastRandom) >= p.cooldown && len(p.randomNext) > 0 {
        song, _ := p.popShuffledRadio(now)
        p.lastRandom = now
        return song, true
    }
    // 2) weighted master queue
    if len(p.queue) == 0 {
        return Song{}, false
    }
    song, ok := p.popShuffledQueue(now)
    return song, ok
}

// popShuffledQueue refills and sorts the master deck if needed, then pops one.
func (p *Playlist) popShuffledQueue(now time.Time) (Song, bool) {
    if p.shuffledQueue == nil || p.shuffledIndex >= len(p.shuffledQueue) {
        p.refillShuffledQueue(now)
    }
    if len(p.shuffledQueue) == 0 {
        return Song{}, false
    }
    s := p.shuffledQueue[p.shuffledIndex]
    p.shuffledIndex++
    p.lastPlayed[s.ID] = now
    return s, true
}

// refillShuffledQueue does a weighted shuffle on queue by age*rand.
func (p *Playlist) refillShuffledQueue(now time.Time) {
    n := len(p.queue)
    type entry struct {
        song Song
        key  float64
    }
    ents := make([]entry, n)
    total := 0.0
    for i, s := range p.queue {
        age := now.Sub(p.lastPlayed[s.ID]).Seconds()
        if age < 1 {
            age = 1
        }
        k := age * p.rng.Float64()
        ents[i] = entry{s, k}
        total += k
    }
    // fallback if all keys zero
    if total == 0 {
        for i := range ents {
            ents[i].key = p.rng.Float64()
        }
    }
    sort.Slice(ents, func(i, j int) bool {
        return ents[i].key > ents[j].key
    })
    p.shuffledQueue = make([]Song, n)
    for i, e := range ents {
        p.shuffledQueue[i] = e.song
    }
    p.shuffledIndex = 0
}

// popShuffledRadio refills/sorts the radio deck, then pops one.
func (p *Playlist) popShuffledRadio(now time.Time) (Song, bool) {
    if p.shuffledRadio == nil || p.shuffledRadioIndex >= len(p.shuffledRadio) {
        p.refillShuffledRadio(now)
    }
    if len(p.shuffledRadio) == 0 {
        return Song{}, false
    }
    s := p.shuffledRadio[p.shuffledRadioIndex]
    p.shuffledRadioIndex++
    p.lastRadioPlayed[s.ID] = now
    return s, true
}

// refillShuffledRadio does a weighted shuffle on randomNext by age*rand.
func (p *Playlist) refillShuffledRadio(now time.Time) {
    n := len(p.randomNext)
    type entry struct {
        song Song
        key  float64
    }
    ents := make([]entry, n)
    total := 0.0
    for i, s := range p.randomNext {
        age := now.Sub(p.lastRadioPlayed[s.ID]).Seconds()
        if age < 1 {
            age = 1
        }
        k := age * p.rng.Float64()
        ents[i] = entry{s, k}
        total += k
    }
    if total == 0 {
        for i := range ents {
            ents[i].key = p.rng.Float64()
        }
    }
    sort.Slice(ents, func(i, j int) bool {
        return ents[i].key > ents[j].key
    })
    p.shuffledRadio = make([]Song, n)
    for i, e := range ents {
        p.shuffledRadio[i] = e.song
    }
    p.shuffledRadioIndex = 0
}

// ForceNextRadioSegment makes the very next Next() call use randomNext.
func (p *Playlist) ForceNextRadioSegment() {
    p.mu.Lock()
    p.forceNextRadio = true
    p.mu.Unlock()
}
