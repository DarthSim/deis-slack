package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type httpHandler struct{}

func validateURL(url, key, msgMAC string) error {
	decMsgMac, err := hex.DecodeString(msgMAC)
	if err != nil {
		return errors.New("Invalid signature")
	}

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(url))
	expectedMAC := mac.Sum(nil)

	log.Println(url)
	log.Println(hex.EncodeToString(expectedMAC))

	if !hmac.Equal(decMsgMac, expectedMAC) {
		return errors.New("Invalid signature")
	}

	return nil
}

func postToSlack(channel, user, app, release string) {
	pretext := os.Getenv("PRETEXT")
	text := strings.TrimSpace(fmt.Sprintf("%s @%s deployed *%s* %s", pretext, user, app, release))

	botName := os.Getenv("BOT_NAME")
	if len(botName) == 0 {
		botName = "Deis Deployer"
	}

	emoji := os.Getenv("EMOJI")
	if len(emoji) == 0 {
		emoji = "nerd_face"
	}

	buf := bytes.NewBuffer([]byte{})
	fmt.Fprintf(
		buf,
		"{\"attachments\": [{\"text\": \"%s\", \"color\": \"good\", \"mrkdwn_in\": [\"text\"]}], \"channel\": \"#%s\", \"username\": \"%s\", \"icon_emoji\": \":%s:\"}",
		text, channel, botName, emoji,
	)

	resp, err := http.Post(os.Getenv("SLACK_URL"), "application/json", buf)
	if err != nil {
		log.Printf("Slack Error: %s\n", err)
	}

	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Slack Error: %s\n", string(body))
		}
	}

	log.Printf("%s: %s", channel, text)
}

func (h httpHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	log.Printf("POST: %s\n", r.URL.String())

	hmacKey := os.Getenv("KEY")
	if len(hmacKey) == 0 {
		log.Println("Error: HMAC key should be defined")
		rw.WriteHeader(403)
		return
	}

	url := *r.URL
	url.Host = r.Host
	url.Scheme = os.Getenv("HTTP_SCHEME")
	if len(url.Scheme) == 0 {
		url.Scheme = "http"
	}

	if err := validateURL(url.String(), hmacKey, r.Header.Get("Authorization")); err != nil {
		log.Printf("Error: %s\n", err)
		rw.WriteHeader(403)
		return
	}

	channel := strings.TrimPrefix(r.URL.Path, "/")
	app := r.URL.Query().Get("app")
	release := r.URL.Query().Get("release")
	user := r.URL.Query().Get("user")

	go postToSlack(channel, user, app, release)

	rw.WriteHeader(200)
}

func main() {
	bind := os.Getenv("BIND")

	s := &http.Server{
		Addr:           bind,
		Handler:        httpHandler{},
		ReadTimeout:    time.Second * 5,
		WriteTimeout:   time.Second * 10,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("Starting server at %s\n", bind)

	log.Fatal(s.ListenAndServe())
}
