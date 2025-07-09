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
	mu           sync.Mutex
	queue        []Song
	currentIndex int // where we are in queue
	randomNext   []Song
	randomIndex  int // where we are in randomNext
	lastRandom   time.Time
	cooldown     time.Duration
	maxChance    float64
	rng          *rand.Rand
	NewSongCh    chan struct{} // signal from Add()

	lastSongID string // to prevent immediate repeats
}

// NewPlaylist seeds its RNG and makes the NewSongCh.
func NewPlaylist(cfg *config.Config) *Playlist {
	src := rand.NewSource(time.Now().UnixNano())
	return &Playlist{
		cooldown:     cfg.RandomCooldown,
		maxChance:    cfg.RandomMaxChance,
		rng:          rand.New(src),
		NewSongCh:    make(chan struct{}, 1),
		currentIndex: 0,
		randomIndex:  0,
		lastSongID:   "",
	}
}

func (p *Playlist) Add(song Song) {
	p.mu.Lock()
	wasEmpty := len(p.queue) == 0
	p.queue = append(p.queue, song)
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
	p.rng.Shuffle(len(p.queue), func(i, j int) {
		p.queue[i], p.queue[j] = p.queue[j], p.queue[i]
	})
	p.currentIndex = 0
}

// Next returns either a randomNext bump or the next queued song.
// It never repeats the same Song.ID twice in a row, and it reshuffles
// the main queue on each wrap‐around.
func (p *Playlist) Next() (Song, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	// 1) randomNext “bonus” cycle
	if now.Sub(p.lastRandom) >= p.cooldown && len(p.randomNext) > 0 {
		// pick the next bump
		song := p.randomNext[p.randomIndex]
		p.randomIndex = (p.randomIndex + 1) % len(p.randomNext)

		// avoid immediate repeat
		if song.ID == p.lastSongID && len(p.randomNext) > 1 {
			song = p.randomNext[p.randomIndex]
			p.randomIndex = (p.randomIndex + 1) % len(p.randomNext)
		}

		p.lastRandom = now
		p.lastSongID = song.ID
		return song, true
	}

	// 2) main queue cycling + shuffle on wrap
	n := len(p.queue)
	if n == 0 {
		return Song{}, false
	}

	nextIdx := p.currentIndex % n

	// if we’re at the start of a new cycle, shuffle & avoid a repeat at pos 0
	if nextIdx == 0 {
		p.rng.Shuffle(n, func(i, j int) {
			p.queue[i], p.queue[j] = p.queue[j], p.queue[i]
		})
		if p.queue[0].ID == p.lastSongID && n > 1 {
			// swap the first with the first non-equal
			for i := 1; i < n; i++ {
				if p.queue[i].ID != p.lastSongID {
					p.queue[0], p.queue[i] = p.queue[i], p.queue[0]
					break
				}
			}
		}
	}

	song := p.queue[nextIdx]
	p.currentIndex = nextIdx + 1

	// record for repeat‐avoidance
	p.lastSongID = song.ID
	return song, true
}
