// client/discord.go
package client

import (
	"fmt"
	"log"

	"github.com/Coop25/CC-Radio/accessor"
	"github.com/Coop25/CC-Radio/config"
	"github.com/Coop25/CC-Radio/manager"
	"github.com/bwmarrin/discordgo"
)

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "addsong",
		Description: "Add a song by url to the master playlist",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "url",
				Description: "The youtube url",
				Required:    true,
			},
		},
	},
	{
		Name:        "add-radio-segment",
		Description: "Add a Radio Segment by url to the radio segment playlist",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "url",
				Description: "The youtube url",
				Required:    true,
			},
		},
	},
	{
		Name:        "addplaylist",
		Description: "Add a playlist by url to the master playlist",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "url",
				Description: "The youtube url",
				Required:    true,
			},
		},
	},
	{
		Name:        "skip",
		Description: "Skip the currently playing song",
	},
	{
		Name:        "saveplaylist",
		Description: "Manually save the current playlist to Pastebin",
	},
	{
		Name:        "deletecurrent",
		Description: "Remove the currently-playing track from the queue",
	},
	{
		Name:        "force-radio-segment",
		Description: "force a radio segment to play next, may take 2 songs",
	},
}

// NewDiscordBot initializes, registers, and opens the Discord session.
func NewDiscordBot(
	cfg *config.Config,
	b *manager.Broadcaster,
	fetcher accessor.Fetcher,
	gist *accessor.GistAccessor,
	pl *accessor.Playlist,
) (*discordgo.Session, error) {

	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("discordgo.New: %w", err)
	}

	if err := dg.Open(); err != nil {
		return nil, fmt.Errorf("dg.Open: %w", err)
	}

	appID := dg.State.User.ID
	guildID := cfg.DiscordGuildID

	// 1) DELETE all existing guild commands
	existing, err := dg.ApplicationCommands(appID, guildID)
	if err != nil {
		log.Printf("⚠️  could not list existing commands: %v", err)
	} else {
		for _, cmd := range existing {
			if err := dg.ApplicationCommandDelete(appID, guildID, cmd.ID); err != nil {
				log.Printf("⚠️  failed to delete command %s: %v", cmd.Name, err)
			} else {
				log.Printf("🗑️  deleted existing command %s", cmd.Name)
			}
		}
	}

	// 2) REGISTER your commands afresh
	for _, cmd := range commands {
		if _, err := dg.ApplicationCommandCreate(appID, guildID, cmd); err != nil {
			log.Printf("❌ cannot create '%s' command: %v", cmd.Name, err)
		} else {
			log.Printf("✅ registered command %s", cmd.Name)
		}
	}

	// Interaction handler
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		data := i.ApplicationCommandData()
		switch data.Name {
		case "addsong":
			songID := data.Options[0].StringValue()
			if err := fetcher.LoadSong(songID); err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ Could not add %q: %v", songID, err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}

			if err := gist.SavePlaylist(pl); err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ Track added - but Save failed: %v", err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("✅ Queued song %q!", songID),
				},
			})
		case "add-radio-segment":
			songID := data.Options[0].StringValue()
			if err := fetcher.LoadRadioSegment(songID); err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ Could not add %q: %v", songID, err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}

			if err := gist.SavePlaylist(pl); err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ RadioSegment added - but Save failed: %v", err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("✅ Queued Segment %q!", songID),
				},
			})

		case "addplaylist":
			songID := data.Options[0].StringValue()
			if err := fetcher.LoadPlaylist(songID); err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ Could not add %q: %v", songID, err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}

			if err := gist.SavePlaylist(pl); err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ Track added - but Save failed: %v", err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("✅ Queued song %q!", songID),
				},
			})

		case "skip":
			b.Skip()
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "⏭️ Skipped current track.",
				},
			})

		case "force-radio-segment":
			pl.ForceNextRadioSegment()
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "⏭️ Forced a radio segment to be played withing the next couple songs.",
				},
			})
		case "saveplaylist":
			if err := gist.SavePlaylist(pl); err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ Save failed: %v", err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "✅ Playlist saved!",
				},
			})
		case "deletecurrent":
			err := b.DeleteCurrent()
			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("❌ %v", err),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			} else {
				if err := gist.SavePlaylist(pl); err != nil {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: fmt.Sprintf("❌ Track removed - but Save failed: %v", err),
							Flags:   discordgo.MessageFlagsEphemeral,
						},
					})
					return
				}
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "✅ Successfully removed the current track from the queue.",
					},
				})
			}
		}

	})

	return dg, nil
}
