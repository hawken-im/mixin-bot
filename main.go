package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fox-one/mixin-sdk-go"
	"github.com/gofrs/uuid"
)

var (
	// Specify the keystore file in the -config parameter
	config = flag.String("config", "", "keystore file path")
	text   = flag.String("text", "hello world", "text message")
)

func main() {
	// Use flag package to parse the parameters
	flag.Parse()

	// Open the keystore file
	f, err := os.Open(*config)
	if err != nil {
		log.Panicln(err)
	}

	// Read the keystore file as json into mixin.Keystore, which is a go struct
	var store mixin.Keystore
	if err := json.NewDecoder(f).Decode(&store); err != nil {
		log.Panicln(err)
	}

	// Create a Mixin Client from the keystore, which is the instance to invoke Mixin APIs
	client, err := mixin.NewFromKeystore(&store)
	if err != nil {
		log.Panicln(err)
	}

	ctx := context.Background()

	me, err := client.UserMe(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	if me.App == nil {
		log.Fatalln("use a bot keystore instead")
	}

	//receiptID := me.App.CreatorID
	receiptID := "74c81e56-85e7-4d19-bcca-ee15cb88d1df"

	sessions, err := client.FetchSessions(ctx, []string{receiptID})
	if err != nil {
		log.Fatalln(err)
	}

	_ = sessions

	req := &mixin.MessageRequest{
		ConversationID: mixin.UniqueConversationID(client.ClientID, receiptID),
		RecipientID:    receiptID,
		MessageID:      mixin.RandomTraceID(),
		Category:       mixin.MessageCategoryPlainText,
		Data:           base64.StdEncoding.EncodeToString([]byte(*text)),
	}

	if err := client.EncryptMessageRequest(req, sessions); err != nil {
		log.Fatalln(err)
	}

	receipts, err := client.SendEncryptedMessages(ctx, []*mixin.MessageRequest{req})
	if err != nil {
		log.Fatalln(err)
	}

	b, _ := json.Marshal(receipts)
	log.Println(string(b))

	h := func(ctx context.Context, msg *mixin.MessageView, userID string) error {
		// if there is no valid user id in the message, drop it
		if userID, _ := uuid.FromString(msg.UserID); userID == uuid.Nil {
			return nil
		}

		// The incoming message's message ID, which is an UUID.
		id, _ := uuid.FromString(msg.MessageID)

		// Create a request
		reply := &mixin.MessageRequest{
			// Reuse the conversation between the sender and the bot.
			// There is an unique UUID for each conversation.
			ConversationID: msg.ConversationID,
			// The user ID of the recipient.
			// The bot will reply messages, so here is the sender's ID of each incoming message.
			RecipientID: msg.UserID,
			// Create a new message id to reply, it should be an UUID never used by any other message.
			// Create it with a "reply" and the incoming message ID.
			MessageID: uuid.NewV5(id, "reply").String(),
			// The bot just reply the same category and the same content of the incoming message
			// So, we copy the category and data
			Category: msg.Category,
			Data:     msg.Data,
		}
		// Send the response
		return client.SendMessage(ctx, reply)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()
	// Start the message loop.
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
			// Pass the callback function into the `BlazeListenFunc`
			if err := client.LoopBlaze(ctx, mixin.BlazeListenFunc(h)); err != nil {
				log.Printf("LoopBlaze: %v", err)
			}
		}
	}
}
