package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	HTTPPort        int           `envconfig:"PORT"        default:"8080"`
	ChunkInterval   time.Duration `envconfig:"CHUNK_INTERVAL" default:"100ms"`
	RandomCooldown  time.Duration `envconfig:"RANDOM_COOLDOWN" default:"30m"`
	RandomMaxChance float64       `envconfig:"RANDOM_MAX_CHANCE" default:"0.1"`

	FetchBaseURL string `envconfig:"FETCH_BASE_URL" required:"true"`
	AuthToken    string `envconfig:"FETCH_AUTH_TOKEN"` // optional

	GITHUB_TOKEN   string        `envconfig:"GITHUB_TOKEN"     required:"true"`
	GITHUB_GIST_ID string        `envconfig:"GITHUB_GIST_ID"`
	SaveInterval   time.Duration `envconfig:"SAVE_INTERVAL" default:"1h"` // how often to auto-save

	DiscordToken   string `envconfig:"DISCORD_TOKEN"   required:"true"`
	DiscordGuildID string `envconfig:"DISCORD_GUILD_ID" required:"true"`

	NowPlayingWebhookURL string `envconfig:"NOW_PLAYING_WEBHOOK_URL"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
