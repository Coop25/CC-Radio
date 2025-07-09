// accessor/fetcher.go
package accessor

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Coop25/CC-Radio/config"
)

// PlaylistItem mirrors one element of "playlist_items"
type PlaylistItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Artist string `json:"artist"` // raw: "MM:SS ArtistName…"
}

// Fetcher knows how to GET raw audio bytes by song ID.
type Fetcher interface {
	FetchBytes(songID string) ([]byte, error)
	LoadPlaylist(playlistURL string) error
	LoadSong(requestURL string) error
}

// httpFetcher implements Fetcher over HTTP.
type httpFetcher struct {
	baseURL  string
	client   *http.Client
	headers  http.Header
	playlist *Playlist
}

// NewHTTPFetcher builds one using your Config.
func NewHTTPFetcher(cfg *config.Config, pl *Playlist) *httpFetcher {
	// shared HTTP client with a reasonable timeout
	cli := &http.Client{Timeout: 10 * time.Second}

	// preset headers for every request
	hdrs := make(http.Header)
	hdrs.Set("User-Agent", "computercraft/1.115.1")
	hdrs.Set("Accept-Charset", "UTF-8")
	hdrs.Set("Connection", "keep-alive")

	return &httpFetcher{
		baseURL:  cfg.FetchBaseURL,
		client:   cli,
		headers:  hdrs,
		playlist: pl,
	}
}

func (h *httpFetcher) FetchBytes(songID string) ([]byte, error) {
	// build URL with ?id=<songID>
	req, err := http.NewRequest("GET", h.baseURL, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("v", "2")
	q.Set("id", songID)
	req.URL.RawQuery = q.Encode()

	// apply shared headers
	req.Header = h.headers

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch bytes: unexpected status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// LoadByID fetches the JSON for a playlist, parses durations and names,
// and adds each Song into h.playlist (then shuffles).
func (h *httpFetcher) LoadPlaylist(playlistURL string) error {
	// build the GET request to the provided URL
	req, err := http.NewRequest("GET", h.baseURL, nil)
	if err != nil {
		return fmt.Errorf("load playlist: %w", err)
	}
	q := req.URL.Query()
	q.Set("v", "2")
	q.Set("search", playlistURL)
	req.URL.RawQuery = q.Encode()
	// apply the same headers as FetchBytes
	req.Header = h.headers

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("load playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("load playlist: status %s, body: %q", resp.Status, body)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading playlist response: %w", err)
	}

	// parse JSON
	var raws []struct {
		PlaylistItems []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Artist string `json:"artist"` // raw: "MM:SS ArtistName…"
		} `json:"playlist_items"`
	}
	if err := json.Unmarshal(data, &raws); err != nil {
		return fmt.Errorf("invalid playlist JSON: %w", err)
	}
	if len(raws) == 0 {
		return fmt.Errorf("no playlist data in response")
	}

	for _, item := range raws[0].PlaylistItems {
		// Artist field starts with "MM:SS " then real artist name
		parts := strings.SplitN(item.Artist, " ", 2)
		if len(parts) < 2 {
			continue
		}
		dur, err := parseDuration(parts[0])
		if err != nil {
			continue
		}

		s := Song{
			ID:       item.ID,
			Name:     item.Name,
			Artist:   parts[1],
			URL:      "", // still only ID→FetchBytes
			Duration: dur,
		}
		h.playlist.Add(s)
	}

	h.playlist.Shuffle()
	return nil
}

func (h *httpFetcher) LoadSong(requestURL string) error {
	log.Printf("[Fetcher] Fetching songs JSON from %s", requestURL)

	req, err := http.NewRequest("GET", h.baseURL, nil)
	if err != nil {
		log.Printf("[Fetcher] NewRequest error: %v", err)
		return fmt.Errorf("load songs: %w", err)
	}
	q := req.URL.Query()
	q.Set("v", "2")
	q.Set("search", requestURL)
	req.URL.RawQuery = q.Encode()

	req.Header = h.headers

	// do request
	resp, err := h.client.Do(req)
	if err != nil {
		log.Printf("[Fetcher] HTTP error: %v", err)
		return fmt.Errorf("load songs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("load songs: status %s, body %q", resp.Status, body)
	}

	// read payload
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Fetcher] ReadAll error: %v", err)
		return fmt.Errorf("reading load songs response: %w", err)
	}

	log.Printf("[Fetcher] Response status: %s, body length: %d", resp.Status, len(data))

	// parse JSON
	var raws []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Artist string `json:"artist"` // "MM:SS � ArtistName"
	}
	if err := json.Unmarshal(data, &raws); err != nil {
		log.Printf("[Fetcher] JSON unmarshal error: %v", err)
		return fmt.Errorf("invalid songs JSON: %w", err)
	}

	// convert & enqueue
	for _, item := range raws {
		dur, err := parseDuration(item.Artist)
		if err != nil {
			log.Printf("[Fetcher] skip %s: invalid duration in %q (%v)", item.ID, item.Artist, err)
			continue
		}
		s := Song{
			ID:       item.ID,
			Name:     item.Name,
			Artist:   item.Artist,
			URL:      "", // still using ID→FetchBytes
			Duration: dur,
		}
		h.playlist.Add(s)
	}

	return nil
}

// parseDuration turns "MM:SS" into time.Duration.
func parseDuration(s string) (time.Duration, error) {
	p := strings.SplitN(s, ":", 2)
	if len(p) != 2 {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	m, err := strconv.Atoi(p[0])
	if err != nil {
		return 0, err
	}
	sec, err := strconv.Atoi(p[1])
	if err != nil {
		return 0, err
	}
	return time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, nil
}
