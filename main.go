package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fox-one/mixin-sdk-go"
	"github.com/gofrs/uuid"
)

const (
	mixinOAuthURL = "https://api.mixin.one/oauth/token"
)

var (
	// Specify the keystore file in the -config parameter
	config          = flag.String("config", "./config/keystore.json", "keystore file path")
	text            = flag.String("text", "hello world", "text message")
	clientSecret    = "8c50cb398bf58f342e5ccf43537004485d1443f66df0241fca492df52bdbdd0a"
	clientIDBot     = "33d79ecd-1293-47cd-b00c-10e889692a3c"
	code            = ""
	token           = ""
	scope           = ""
	responseContent = ""
)

type responseFromAuth struct {
	Data InData `json:"data"`
}

type InData struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
}

func main() {
	ctx := context.Background()
	// Use flag package to parse the parameters
	flag.Parse()

	// Open the keystore file
	f, err := os.Open(*config)
	if err != nil {
		log.Panicln(err)
	}

	//Run a http thread until get the code.
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/oauth", oauthHandler)

	go http.ListenAndServe(":8000", nil) //解决部署到服务器上配置nginx 端口8000的问题

	for {
		if code != "" {
			break
		}
		time.Sleep(1 * time.Second)
		log.Println("Code geto:", code)
	}

	for {
		if responseContent != "" {
			break
		}
		time.Sleep(1 * time.Second)
		log.Println("response content geto:", responseContent)
	}
	var jsonData responseFromAuth

	err = json.Unmarshal([]byte(responseContent), &jsonData) // here!

	if err != nil {
		panic(err)
	}

	// test struct data
	fmt.Println(jsonData)
	token = jsonData.Data.AccessToken
	scope = jsonData.Data.Scope
	fmt.Println("token is", token)
	fmt.Println("scope is", scope)
	client1 := mixin.NewFromAccessToken(token)

	//TODO//
	//USE Token to get some authority

	// Read the keystore file as json into mixin.Keystore, which is a go struct
	var store mixin.Keystore
	if err := json.NewDecoder(f).Decode(&store); err != nil {
		log.Panicln(err)
	}
	clientBot, err := mixin.NewFromKeystore(&store)
	if err != nil {
		log.Panicln(err)
	}

	log.Println("client id", clientBot.ClientID)

	me, err := clientBot.UserMe(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	if me.App == nil {
		log.Fatalln("use a bot keystore instead")
	}

	//receiptID := me.App.CreatorID
	receiptID := "559c2f11-8e77-44b8-8b86-be898fad5e47"
	sessions, err := clientBot.FetchSessions(ctx, []string{receiptID})
	if err != nil {
		log.Fatalln(err)
	}

	_ = sessions

	req := &mixin.MessageRequest{
		ConversationID: mixin.UniqueConversationID(clientBot.ClientID, receiptID),
		RecipientID:    receiptID,
		MessageID:      mixin.RandomTraceID(),
		Category:       mixin.MessageCategoryPlainText,
		Data:           base64.StdEncoding.EncodeToString([]byte(*text)),
	}

	if err := clientBot.EncryptMessageRequest(req, sessions); err != nil {
		log.Fatalln(err)
	}

	receipts, err := clientBot.SendEncryptedMessages(ctx, []*mixin.MessageRequest{req})
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
			Category: mixin.MessageCategoryPlainText,
			Data:     base64.StdEncoding.EncodeToString([]byte("haha")),
		}
		// Send the response
		return client1.SendMessage(ctx, reply)
	}

	_, stop := signal.NotifyContext( //before, it was ctx,stop := blabla
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
			if err := client1.LoopBlaze(ctx, mixin.BlazeListenFunc(h)); err != nil {
				log.Printf("LoopBlaze: %v", err)
			}
		}
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	_url := "https://mixin.one/oauth/authorize?client_id=%s&scope=%s&response_type=code&return_to=%s"
	return_to := ""
	_url = fmt.Sprintf(_url, clientIDBot, "PROFILE:READ", return_to)
	http.Redirect(w, r, _url, http.StatusFound)
}

func oauthHandler(w http.ResponseWriter, r *http.Request) {

	query := r.URL.Query()
	code = query.Get("code")
	if len(code) != 64 {
		fmt.Fprintf(w, "invalid code: %s", code)
		return
	}

	payload := fmt.Sprintf(
		`{"client_id": "%s", "code": "%s", "client_secret": "%s"}`,
		clientIDBot, code, clientSecret,
	)

	client := http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("POST", mixinOAuthURL, bytes.NewBufferString(payload))
	if err != nil {
		msg := fmt.Sprintf("ERROR new http request failed: %s", err)
		fmt.Printf("%s", msg)
		fmt.Fprint(w, msg)
		return
	}
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("ERROR POST %s failed: %s", mixinOAuthURL, err)
		fmt.Printf("%s", msg)
		fmt.Fprint(w, msg)
		return
	}

	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		msg := fmt.Sprintf("ERROR read response failed: %s", err)
		fmt.Printf("%s", msg)
		fmt.Fprint(w, msg)
		return
	}
	fmt.Println(string(content))
	fmt.Fprint(w, string(content))
	responseContent = string(content)
}
