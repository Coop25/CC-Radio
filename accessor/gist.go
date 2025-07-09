// accessor/gist.go
package accessor

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "github.com/Coop25/CC-Radio/config"
)

// playlistBackup matches the snapshot format
type playlistBackup struct {
    Queue      []Song `json:"queue"`
    RandomNext []Song `json:"random_next"`
}

type GistAccessor struct {
    token  string
    gistID string
    client *http.Client
}

func NewGistAccessor(cfg *config.Config) *GistAccessor {
    return &GistAccessor{
        token:  cfg.GITHUB_TOKEN,
        gistID: cfg.GITHUB_GIST_ID,
        client: &http.Client{Timeout: 10 * time.Second},
    }
}

// SavePlaylist PATCHes the existing gist, replacing playlist.json
func (g *GistAccessor) SavePlaylist(pl *Playlist) error {
    // take a snapshot
    pl.mu.Lock()
    backup := playlistBackup{
        Queue:      append([]Song(nil), pl.queue...),
        RandomNext: append([]Song(nil), pl.randomNext...),
    }
    pl.mu.Unlock()

    blob, err := json.MarshalIndent(backup, "", "  ")
    if err != nil {
        return err
    }

    payload := map[string]interface{}{
        "files": map[string]map[string]string{
            "playlist.json": {
                "content": string(blob),
            },
        },
    }
    body, _ := json.Marshal(payload)

    url := fmt.Sprintf("https://api.github.com/gists/%s", g.gistID)
    req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "token "+g.token)
    req.Header.Set("Accept", "application/vnd.github.v3+json")

    resp, err := g.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 300 {
        data, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("gist update failed: %s", strings.TrimSpace(string(data)))
    }
    return nil
}

// LoadByID GETs the raw file from raw.githubusercontent.com
func (g *GistAccessor) LoadByID(pl *Playlist) error {
    rawURL := fmt.Sprintf(
        "https://gist.githubusercontent.com/%s/raw/playlist.json",
        g.gistID,
    )
    resp, err := g.client.Get(rawURL)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        data, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("gist load failed: %s", string(data))
    }

    data, err := io.ReadAll(resp.Body)
    if err != nil {
        return err
    }

    var backup playlistBackup
    if err := json.Unmarshal(data, &backup); err != nil {
        return fmt.Errorf("invalid JSON in gist: %w", err)
    }

    pl.mu.Lock()
    pl.queue = backup.Queue
    pl.randomNext = backup.RandomNext
    pl.mu.Unlock()
    pl.Shuffle()
    return nil
}
