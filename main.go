package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func handleAppMentionEvent(event *slackevents.AppMentionEvent, client *slack.Client) error {

	user, err := client.GetUserInfo(event.User)
	if err != nil {
		return err
	}
	text := strings.ToLower(event.Text)

	attachment := slack.Attachment{}
	attachment.Fields = []slack.AttachmentField{
		{
			Title: "Date",
			Value: time.Now().String(),
		}, {
			Title: "Initializer",
			Value: user.Name,
		},
	}
	if strings.Contains(text, "hello") {
		attachment.Text = fmt.Sprintf("Hello %s", user.Name)
		attachment.Pretext = "Greetings"
		attachment.Color = "#4af030"
	} else {
		attachment.Text = fmt.Sprintf("How can I help you %s?", user.Name)
		attachment.Pretext = "How can I be of service"
		attachment.Color = "#3d3d3d"
	}
	_, _, err = client.PostMessage(event.Channel, slack.MsgOptionAttachments(attachment))
	if err != nil {
		return fmt.Errorf("failed to post message: %w", err)
	}
	return nil
}

func handleHelloCommand(command slack.SlashCommand, client *slack.Client) error {
	attachment := slack.Attachment{}
	attachment.Fields = []slack.AttachmentField{
		{
			Title: "Date",
			Value: time.Now().String(),
		}, {
			Title: "Initializer",
			Value: command.UserName,
		},
	}
	attachment.Text = fmt.Sprintf("Hello %s", command.Text)
	attachment.Color = "#4af030"

	_, _, err := client.PostMessage(command.ChannelID, slack.MsgOptionAttachments(attachment))
	if err != nil {
		return fmt.Errorf("failed to post message: %w", err)
	}
	return nil
}

// handleSlashCommand handles slash commands by routing them to the appropriate function
func handleSlashCommand(command slack.SlashCommand, client *slack.Client) error {
	log.Println("Received slash command", command)
	switch command.Command {
	case "/hello":
		return handleHelloCommand(command, client)
	}
	return nil
}

// handleEventMessage will take an event and handle itproperly based on the event type
func handleEventMessage(event slackevents.EventsAPIEvent, client *slack.Client) error {
	switch event.Type {

	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			err := handleAppMentionEvent(ev, client)
			if err != nil {
				return err
			}
		}
	default:
		return errors.New("unsupported event type")
	}
	return nil
}

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
					err := handleEventMessage(eventsApiEvent, client)
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
					log.Println("Received slash event: ", command)
					socketClient.Ack(*event.Request)
					err := handleSlashCommand(command, client)
					if err != nil {
						log.Fatal(err)
					}
				default:
					log.Println("****** Received Event: ", event)
				}
			}
		}
	}(ctx, client, sockentClient)
	sockentClient.Run()
}
