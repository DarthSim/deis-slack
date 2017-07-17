package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var (
	debug    bool
	bind     string
	hmacKey  []byte
	botName  string
	emoji    string
	pretext  string
	hookURLs map[string]string
)

func validateURL(url, msgMAC string) error {
	decMsgMac, err := hex.DecodeString(msgMAC)
	if err != nil {
		return errors.New("Invalid signature")
	}

	mac := hmac.New(sha1.New, hmacKey)
	mac.Write([]byte(url))
	expectedMAC := mac.Sum(nil)

	if debug {
		log.Printf("Requested URL: %s\n", url)
		log.Printf("Provided HMAC: %s\n", msgMAC)
		log.Printf("Expected HMAC: %s\n", hex.EncodeToString(expectedMAC))
	}

	if !hmac.Equal(decMsgMac, expectedMAC) {
		return errors.New("Invalid signature")
	}

	return nil
}

func postToSlack(user, app, release string) {
	url, ok := hookURLs[app]
	if !ok {
		log.Printf("Error: Unknown application %s", app)
		return
	}

	text := strings.TrimSpace(fmt.Sprintf("%s @%s deployed *%s* %s", pretext, user, app, release))

	buf := bytes.NewBuffer([]byte{})
	fmt.Fprintf(
		buf,
		"{\"attachments\": [{\"text\": \"%s\", \"color\": \"good\", \"mrkdwn_in\": [\"text\"]}], \"username\": \"%s\", \"icon_emoji\": \":%s:\"}",
		text, botName, emoji,
	)

	resp, err := http.Post(url, "application/json", buf)
	if err != nil {
		log.Printf("Slack Error: %s\n", err)
	}

	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Slack Error: %s\n", string(body))
		}
	}

	log.Printf("Posted to Slack: %s\n", text)
}

func requestHandler(rw http.ResponseWriter, r *http.Request) {
	log.Printf("POST: %s\n", r.URL.String())

	url := *r.URL
	url.Host = r.Host
	url.Scheme = os.Getenv("HTTP_SCHEME")
	if len(url.Scheme) == 0 {
		url.Scheme = "http"
	}

	if err := validateURL(url.String(), r.Header.Get("Authorization")); err != nil {
		log.Printf("Error: %s\n", err)
		rw.WriteHeader(403)
		return
	}

	app := r.URL.Query().Get("app")
	release := r.URL.Query().Get("release")
	user := r.URL.Query().Get("user")

	go postToSlack(user, app, release)

	rw.WriteHeader(200)
}

func parseHookUrls(s string) (urls map[string]string) {
	urls = make(map[string]string)

	if len(s) == 0 {
		return
	}

	lines := strings.Split(s, ",")
	for _, l := range lines {
		parts := strings.Split(l, "=")
		if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) == 0 {
			continue
		}

		ha := strings.TrimSpace(parts[0])
		hu := strings.TrimSpace(parts[1])

		if _, err := url.Parse(hu); err != nil {
			if debug {
				log.Printf("Skipping %s; Invalid url", hu)
			}
			continue
		}
		urls[ha] = hu
	}

	return
}

func init() {
	debug = len(os.Getenv("DEBUG")) > 0

	bind = os.Getenv("BIND")

	hmacKey = []byte(os.Getenv("KEY"))
	if len(hmacKey) == 0 {
		log.Println("WARNING! HMAC key is emty")
	}

	botName = os.Getenv("BOT_NAME")
	if len(botName) == 0 {
		botName = "Deis Deployer"
	}

	emoji = os.Getenv("EMOJI")
	if len(emoji) == 0 {
		emoji = "nerd_face"
	}

	pretext = os.Getenv("PRETEXT")

	hookURLs = parseHookUrls(os.Getenv("HOOK_URLS"))

	if debug {
		log.Printf("Bot name:   %s\n", botName)
		log.Printf("Emoji icon: :%s:\n", emoji)
		log.Printf("Pretext:    %s\n", pretext)

		log.Println("Hook URLs:")
		for k, v := range hookURLs {
			log.Printf(" -> %s: %s\n", k, v)
		}
	}
}

func main() {
	http.HandleFunc("/", requestHandler)
	log.Printf("Starting server at %s\n", bind)
	log.Fatal(http.ListenAndServe(bind, nil))
}
