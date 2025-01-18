package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
)

var (
	port         = flag.Int("port", 8000, "Server port. Defaults to 8000.")
	webhook      = flag.String("webhook", "", "URL to send message webhooks. Must supply a value.")
	mediaDir     = flag.String("media", "", "Media storage directory. Defaults to a random temporary file.")
	snapshotPath = flag.String("snapshot", "", "Path of the snapshot to load.")
)

type Server struct {
	mux   sync.RWMutex
	Chats []Chat `json:"chats"`
}

type Chat struct {
	User     string    `json:"user"`
	Messages []Message `json:"messages"`
}

type Message struct {
	To   string       `json:"to"`
	Type string       `json:"type"`
	Text *TextMessage `json:"text,omitempty"`
}

type TextMessage struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

func main() {
	// http.HandleFunc("POST /{phone}/messages")
	flag.CommandLine.Usage = usage
	if *webhook == "" {
		fmt.Fprintf(os.Stderr, "error: Must supply a webhook url.")
		usage()
		os.Exit(1)
	}

	log.Fatal(http.ListenAndServe(":8000", nil))
}

func usage() {
	fmt.Fprintf(os.Stderr, "USAGE: %s [flag]...\nAvailable flags:\n", os.Args[0])
	flag.PrintDefaults()
}
