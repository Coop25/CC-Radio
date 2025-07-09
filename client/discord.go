// client/discord.go
package client

import (
	"fmt"
	"os"

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
		Name:        "addRadioSegment",
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
		Name:        "forceSegment",
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
		case "addRadioSegment":
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

		case "forceSegment":
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

	if err := dg.Open(); err != nil {
		return nil, fmt.Errorf("dg.Open: %w", err)
	}

	appID := dg.State.User.ID
	for _, cmd := range commands {
		if _, err := dg.ApplicationCommandCreate(appID, cfg.DiscordGuildID, cmd); err != nil {
			fmt.Fprintf(os.Stderr, "unable to create command %s: %v\n", cmd.Name, err)
		}
	}

	return dg, nil
}
