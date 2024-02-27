package main

import (
	"context"
	"log"
	"os"
	handlers "talaniz/slack-bot/lib"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func main() {
	godotenv.Load(".env")

	token := os.Getenv("SLACK_BOT_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")

	client := slack.New(token, slack.OptionDebug(true), slack.OptionAppLevelToken(appToken))
	sockentClient := socketmode.New(
		client,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	// Implement a graceful shutdown here
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func(ctx context.Context, client *slack.Client, socketClient *socketmode.Client) {
		for {
			select {
			case <-ctx.Done():
				log.Println("Shutting down socketmode listener")
				return
			case event := <-socketClient.Events:
				switch event.Type {
				// Slack events
				case socketmode.EventTypeEventsAPI:
					eventsApiEvent, ok := event.Data.(slackevents.EventsAPIEvent)
					if !ok {
						log.Printf("Could not type case the event to the EventsAPIEvent: %v\n", event)
						continue
					}
					log.Println("Received API event: ", event.Type)
					socketClient.Ack(*event.Request)
					err := handlers.HandleEventMessage(eventsApiEvent, client)
					if err != nil {
						log.Fatal(err)
					}
				// Slash command
				case socketmode.EventTypeSlashCommand:
					command, ok := event.Data.(slack.SlashCommand)
					if !ok {
						log.Printf("Could not type case the message to a SlashCommand: %v\n", command)
						continue
					}

					// consider removing the client parameter as it's unused
					payload, err := handlers.HandleSlashCommand(command, client)
					if err != nil {
						log.Fatal(err)
					}
					socketClient.Ack(*event.Request, payload)
				case socketmode.EventTypeInteractive:
					interaction, ok := event.Data.(slack.InteractionCallback)
					if !ok {
						log.Printf("Could not type cast the message to an Interaction callback: %v\n", interaction)
						continue
					}

					err := handlers.HandleInteractionEvent(interaction, client)
					if err != nil {
						log.Fatal(err)
					}
					socketClient.Ack(*event.Request)
				default:
					log.Println("****** Received Event: ", event)
				}
			}
		}
	}(ctx, client, sockentClient)
	sockentClient.Run()
}
