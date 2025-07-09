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

type gistFile struct {
    Content string `json:"content"`
}

type gistResponse struct {
    Files map[string]gistFile `json:"files"`
}

func (g *GistAccessor) LoadByID(pl *Playlist) error {
    // 1) Call the GitHub API to get the Gist JSON
    url := fmt.Sprintf("https://api.github.com/gists/%s", g.gistID)
    req, err := http.NewRequest("GET", url, nil)
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

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("gist load failed: %s", resp.Status)
    }

    // 2) Decode the response into our struct
    var gr gistResponse
    if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
        return fmt.Errorf("invalid Gist JSON: %w", err)
    }

    // 3) Extract the content of "playlist.json"
    file, ok := gr.Files["playlist.json"]
    if !ok {
        return fmt.Errorf("gist does not contain playlist.json")
    }

    // 4) Unmarshal that content into our backup struct
    var backup playlistBackup
    if err := json.Unmarshal([]byte(file.Content), &backup); err != nil {
        return fmt.Errorf("invalid playlist JSON in gist: %w", err)
    }

    // 5) Replace the playlist state
    pl.mu.Lock()
    pl.queue = backup.Queue
    pl.randomNext = backup.RandomNext
    pl.mu.Unlock()

    // 6) Shuffle and return
    pl.Shuffle()
    return nil
}