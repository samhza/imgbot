package main

import (
	"context"
	"flag"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
	"github.com/pelletier/go-toml"
)

type Config struct {
	Token              string
	AppID              discord.AppID
	ImagesDir          string
	Content            string
	ActivityName       string
	CommandName        string
	CommandDescription string
	PostChannel        discord.ChannelID
	PostInterval       string
}

func main() {
	cfgpath := flag.String("c", "imgbot.toml", "path to bot config")
	flag.Parse()
	f, err := os.Open(*cfgpath)
	if err != nil {
		log.Fatalln(err)
	}
	var cfg Config
	err = toml.NewDecoder(f).Decode(&cfg)
	if err != nil {
		log.Fatalln(err)
	}

	interval, err := time.ParseDuration(cfg.PostInterval)
	if err != nil {
		log.Fatalln("parsing PostInterval:", err)
	}

	ses, err := session.New("Bot " + cfg.Token)
	if err != nil {
		log.Fatalln(err)
	}
	cmds, err := ses.BulkOverwriteGuildCommands(cfg.AppID, 388492601365495809, []discord.Command{{
		Name:        cfg.CommandName,
		Description: cfg.CommandDescription,
		Type:        discord.ChatInputCommand,
	}})
	if err != nil {
		log.Fatalln("creating commands:", err)
	}
	cmdid := cmds[0].ID
	ses.AddHandler(func(ev *gateway.InteractionCreateEvent) {
		if ev.Data.Type() != discord.CommandInteraction {
			return
		}
		data := ev.Data.(*discord.CommandInteractionData)
		if data.ID != cmdid {
			return
		}
		fname, f, err := randomImage(cfg.ImagesDir)
		if err != nil {
			ses.RespondInteraction(ev.ID, ev.Token, api.InteractionResponse{
				Type: api.MessageInteractionWithSource,
				Data: &api.InteractionResponseData{
					Flags:   api.EphemeralResponse,
					Content: option.NewNullableString(err.Error()),
				},
			})
			return
		}
		defer f.Close()
		err = ses.RespondInteraction(ev.ID, ev.Token, api.InteractionResponse{
			Type: api.MessageInteractionWithSource,
			Data: &api.InteractionResponseData{
				Files: []sendpart.File{{Name: fname, Reader: f}},
			},
		})
		if err != nil {
			log.Println("responding to interaction:", err)
		}
	})
	if err := ses.Open(context.Background()); err != nil {
		log.Fatalln("connecting to gateway:", err)
	}
	log.Println("connected to discord")
	err = ses.UpdateStatus(gateway.UpdateStatusData{
		Activities: []discord.Activity{{Name: cfg.ActivityName}},
	})
	if err != nil {
		log.Fatalln("setting status:", err)
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	for {
		select {
		case <-sigs:
			log.Println("signal received")
			os.Exit(0)
		case <-waitInterval(interval):
			fname, f, err := randomImage(cfg.ImagesDir)
			if err != nil {
				log.Println("getting random image:", err)
				continue
			}
			content := ""
			if cfg.Content != "" {
				content = strings.ReplaceAll(cfg.Content, "%filename%", fname)
			}
			_, err = ses.SendMessageComplex(cfg.PostChannel,
				api.SendMessageData{
					Content: content,
					Files:   []sendpart.File{{Name: fname, Reader: f}},
				})
			f.Close()
			if err != nil {
				log.Println("sending message:", err)
			}
		}
	}
}

func waitInterval(int time.Duration) <-chan time.Time {
	now := time.Now()
	dur := now.Truncate(int).Add(int).Sub(now)
	return time.After(dur)
}

func randomImage(dir string) (string, *os.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	fname := entries[rand.Intn(len(entries))].Name()
	f, err := os.Open(filepath.Join(dir, fname))
	if err != nil {
		return "", nil, err
	}
	return fname, f, err
}
