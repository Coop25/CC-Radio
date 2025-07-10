package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

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
	pl := accessor.NewPlaylist(cfg)
	fetcher := accessor.NewHTTPFetcher(cfg, pl)
	gist := accessor.NewGistAccessor(cfg)

	// load existing state from Gist
	if err := gist.LoadByID(pl); err != nil {
		log.Fatalf("load from Gist failed: %v", err)
	}
	log.Println("✅ loaded playlist from Gist")

	// 3) init broadcaster & HTTP
	b := manager.NewBroadcaster(cfg, pl, fetcher)
	b.Start(context.Background())

	client.RegisterWS(b)
	// 6) Instantiate Discord bot just like everything else
	dg, err := client.NewDiscordBot(cfg, b, fetcher, gist, pl)
	if err != nil {
		log.Fatalf("Discord bot init failed: %v", err)
	}
	defer dg.Close()

	log.Printf("listening on :%d", cfg.HTTPPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.HTTPPort), nil))
}
