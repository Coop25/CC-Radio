package accessor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Coop25/CC-Radio/config"
)

// backup holds both parts of your playlist state.
type playlistBackup struct {
	Queue      []Song `json:"queue"`
	RandomNext []Song `json:"random_next"`
}

type PastebinAccessor struct {
	devKey string
	client *http.Client
}

func NewPastebinAccessor(cfg *config.Config) *PastebinAccessor {
	return &PastebinAccessor{
		devKey: cfg.PastebinDevKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// SavePlaylist creates an unlisted paste (api_paste_private=1, never expires)
// and returns just the paste ID (e.g. "ABCDEFG").
func (p *PastebinAccessor) SavePlaylist(pl *Playlist, title string) (string, error) {
	pl.mu.Lock()
	backup := playlistBackup{
		Queue:      append([]Song(nil), pl.queue...),
		RandomNext: append([]Song(nil), pl.randomNext...),
	}
	pl.mu.Unlock()

	blob, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("api_dev_key", p.devKey)
	form.Set("api_option", "paste")
	form.Set("api_paste_code", string(blob))
	form.Set("api_paste_name", title)
	form.Set("api_paste_private", "1")     // unlisted
	form.Set("api_paste_expire_date", "N") // never expire

	resp, err := p.client.Post(
		"https://pastebin.com/api/api_post.php",
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pastebin error: %s", string(body))
	}

	u, err := url.Parse(strings.TrimSpace(string(body)))
	if err != nil {
		return "", fmt.Errorf("invalid paste URL %q: %w", string(body), err)
	}
	return path.Base(u.Path), nil
}

// LoadByID fetches your raw paste at /raw/<pasteID>, restores both slices,
// and reshuffles the master queue.
func (p *PastebinAccessor) LoadByID(pasteID string, pl *Playlist) error {
	rawURL := fmt.Sprintf("https://pastebin.com/raw/%s", pasteID)
	resp, err := p.client.Get(rawURL)
	if err != nil {
		return fmt.Errorf("loading paste %s: %w", pasteID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pastebin load error: %s %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var backup playlistBackup
	if err := json.Unmarshal(body, &backup); err != nil {
		return fmt.Errorf("invalid JSON in paste: %w", err)
	}

	pl.mu.Lock()
	pl.queue = backup.Queue
	pl.randomNext = backup.RandomNext
	pl.mu.Unlock()

	pl.Shuffle()
	return nil
}
