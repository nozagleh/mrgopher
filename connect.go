package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// SessionStartLimit ...
type SessionStartLimit struct {
	Total      int
	Remaining  int
	ResetAfter int
}

// DiscordEndpoint ...
type DiscordEndpoint struct {
	URL               string
	Shards            int
	SessionStartLimit SessionStartLimit
}

// Author ...
type Author struct {
	UserName      string `json:"username"`
	ID            string `json:"id"`
	Discriminator string `json:"discriminator"`
	Bot           bool   `json:"bot"`
	Avatar        string `json:"avatar"`
}

// Content ...
type Content struct {
	Type    string `json:"type"`
	Author  Author `json:"author"`
	Content string `json:"content"`
}

// Message ...
type Message struct {
	T  string  `json:"t"`
	S  int     `json:"s"`
	OP int     `json:"op"`
	D  Content `json:"d"`
}

// Properties ...
type Properties struct {
	OS      string `json:"$os,omitempty"`
	Browser string `json:"$browser,omitempty"`
	Device  string `json:"$device,omitempty"`
}

// IdentData ...
type IdentData struct {
	Token      string      `json:"token"`
	Properties *Properties `json:"properties,omitempty"`
}

// Identify ..
type Identify struct {
	OP int       `json:"op"`
	D  IdentData `json:"d"`
}

// Heartbeat ...
type Heartbeat struct {
	OP int         `json:"op"`
	D  interface{} `json:"d"`
}

var out DiscordEndpoint
var inmsg, outmsg Message
var heartbeat Heartbeat

func establish() (*http.Response, error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET", "https://discordapp.com/api/v6/gateway/bot", nil)
	if err != nil {
		log.Println("new req:", err)
		return nil, err
	}
	req.Header.Add("Authorization", "NTA3OTkyMTk0OTUxNDEzNzcw.Ds4SLA.wmq121KPJlzkHOVUelXKbs0DxhA")

	resp, err := client.Do(req)
	if err != nil {
		log.Println("do:", err)
		return nil, err
	}

	log.Printf("response: %s", string(resp.Status))

	return resp, err
}

func sendMessage() {
	client := &http.Client{}
	var jsonStr = []byte(`{"content":"Buy cheese and bread for breakfast."}`)

	req, err := http.NewRequest("POST", "https://discordapp.com/api/channels/305512054478077952/messages", bytes.NewBuffer(jsonStr))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bot NTA3OTkyMTk0OTUxNDEzNzcw.Ds4SLA.wmq121KPJlzkHOVUelXKbs0DxhA")
	req.Header.Add("User-Agent", "herraBot (https://nozagleh.com/@herrabot, 0.1)")

	response, err := client.Do(req)
	if err != nil {
		log.Println("sending:", err)
	}
	log.Println("resp:", response)
}

func main() {
	resp, err := establish()
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		err2 := json.Unmarshal(b, &out)
		if err2 != nil {
			log.Println("unmarshal:", err2)
		}
	}

	// Set the address to the Discord Gateway
	var addr = flag.String("addr", strings.TrimPrefix(out.URL, "wss://"), "ws endpoint")

	// Parse the set flags
	flag.Parse()
	log.SetFlags(0)

	// Make a channel via os.Signal and get the value as interrupt
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Set the URL to the WSs
	u := url.URL{Scheme: "wss", Host: *addr, Path: "/", RawQuery: "v=6&encoding=json"}
	log.Printf("connecting to %s", u.String())

	// Dialing the WebSocket endpoint
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	// Defer close on main() end
	defer c.Close()

	done := make(chan Message)

	// Anonymous goroutine func
	go func() {
		// defer close the channel
		defer close(done)
		// Loop for messages
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			err2 := json.Unmarshal(message, &inmsg)
			if err2 != nil {
				log.Println("unmarshal:", err2)
			}
			done <- inmsg
			// Log a successful message
			log.Printf("recv: %s", message)
		}
	}()

	// Create a ticker
	ticker := time.NewTicker(41250 * time.Millisecond)
	// Stop ticker on exit (defer)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			if inmsg.OP == 10 {
				r, err := json.Marshal(Identify{2, IdentData{"NTA3OTkyMTk0OTUxNDEzNzcw.Ds4SLA.wmq121KPJlzkHOVUelXKbs0DxhA", &Properties{}}})
				if err != nil {
					log.Printf("ident: %s", err)
					return
				}

				log.Println("r:", string(r))
				err2 := c.WriteMessage(websocket.TextMessage, r)
				if err2 != nil {
					log.Printf("send ident: %s", err2)
					return
				}
			} else if inmsg.OP == 0 && inmsg.T == "MESSAGE_CREATE" && inmsg.D.Author.Bot != true && strings.HasPrefix(inmsg.D.Content, "go ") {
				log.Printf("MESSAGE CREATED BY USER: %s", inmsg.T)
				sendMessage()
			}
		case <-ticker.C:
			var h []byte
			var err error
			log.Printf("INMSG S: %d", inmsg.S)
			s := inmsg.S
			if s == 0 {
				h, err = json.Marshal(Heartbeat{1, nil})
			} else {
				h, err = json.Marshal(Heartbeat{1, inmsg.S})
			}

			if err != nil {
				log.Println("write:", err)
				return
			}

			log.Println(string(h))
			err2 := c.WriteMessage(websocket.TextMessage, h)
			if err2 != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			log.Println("interrupted")

			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
				log.Print("Second done")
			case <-time.After(time.Second):
				log.Print("Time after")
			}
			return
		}
	}
}
