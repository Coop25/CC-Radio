package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Coop25/CC-Radio/accessor"
	"github.com/Coop25/CC-Radio/client"
	"github.com/Coop25/CC-Radio/config"
	"github.com/Coop25/CC-Radio/manager"
)

func main() {
	// 1) load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// 2) init playlist
	pl := accessor.NewPlaylist(cfg.RandomCooldown, cfg.RandomMaxChance)
	fetcher := accessor.NewHTTPFetcher(cfg, pl)

	// Initialize Pastebin accessor
	pb := accessor.NewPastebinAccessor(cfg)

	// If a paste ID was provided, load it now
	if cfg.PastebinPasteID != "" {
		if err := pb.LoadByID(cfg.PastebinPasteID, pl); err != nil {
			log.Fatalf("failed to load playlist from paste ID %s: %v", cfg.PastebinPasteID, err)
		}
		fmt.Println("Loaded playlist from paste:", cfg.PastebinPasteID)
	}

	// 3) init broadcaster & HTTP
	b := manager.NewBroadcaster(cfg.ChunkInterval, pl, fetcher)
	b.Start(context.Background())

	client.RegisterWS(b)
	// 6) Instantiate Discord bot just like everything else
	dg, err := client.NewDiscordBot(cfg, b, fetcher)
	if err != nil {
		log.Fatalf("Discord bot init failed: %v", err)
	}
	defer dg.Close()

	// 6) Launch auto-save goroutine
	go func() {
		ticker := time.NewTicker(cfg.SaveInterval)
		defer ticker.Stop()
		for {
			<-ticker.C
			pasteID, err := pb.SavePlaylist(pl, "Auto-saved Playlist")
			if err != nil {
				log.Printf("⚠️  auto-save failed: %v", err)
				continue
			}
			log.Printf("✅  playlist auto-saved to paste ID %q", pasteID)
		}
	}()

	log.Printf("listening on :%d", cfg.HTTPPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.HTTPPort), nil))
}
