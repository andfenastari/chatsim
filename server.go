package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

var (
	port         = flag.Int("port", 8000, "Server port. Defaults to '8000'.")
	webhook      = flag.String("webhook", "", "URL to send message webhooks. Must supply a value.")
	mediaDir     = flag.String("media", "", "Media storage directory. Defaults to a random temporary file.")
	snapshotPath = flag.String("snapshot", "", "Path of the snapshot to load.")
	agentPhone   = flag.String("agent-phone", "+00", "Phone number to use for the agent. Defaults to '+00'")
)

type Server struct {
	http.ServeMux
	State *State
}

func main() {
	flag.CommandLine.Usage = usage
	// if *webhook == "" {
	// 	fmt.Fprintf(os.Stderr, "error: Must supply a webhook url.")
	// 	usage()
	// 	os.Exit(1)
	// }

	server := newServer(&State{})

	log.Fatal(http.ListenAndServe(":8000", server))
}

func usage() {
	fmt.Fprintf(os.Stderr, "USAGE: %s [flag]...\nAvailable flags:\n", os.Args[0])
	flag.PrintDefaults()
}

func newServer(state *State) *Server {
	server := &Server{State: state}
	server.HandleFunc("POST /{sender}/messages", server.handleCreateMessage)
	return server
}

type CreateMessageResponse struct {
	MessagingProduct string                         `json:"messaging_product"`
	Contacts         []CreateMessageResponseContact `json:"contacts"`
	Messages         []CreateMessageResponseMessage `json:"messages"`
}

type CreateMessageResponseContact struct {
	Input string `json:"input"`
	WaId  string `json:"wa_id"`
}

type CreateMessageResponseMessage struct {
	Id string `json:"id"`
}

func newCreateMessageResponse(user string) *CreateMessageResponse {
	return &CreateMessageResponse{
		MessagingProduct: "whatsapp",
		Contacts: []CreateMessageResponseContact{
			{Input: user, WaId: user},
		},
		Messages: []CreateMessageResponseMessage{
			{Id: "FAKE_ID"},
		},
	}
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, req *http.Request) {
	log.Print("Gottem")
	var err error

	sender := req.PathValue("sender")

	var message *Message
	err = json.NewDecoder(req.Body).Decode(&message)
	if err != nil {
		log.Printf("Decoding create message body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var chatUser string
	if sender == *agentPhone {
		chatUser = message.To
	} else {
		chatUser = sender
	}

	s.State.mux.Lock()
	s.State.AddMessage(chatUser, message)
	s.State.mux.Unlock()

	json.NewEncoder(w).Encode(newCreateMessageResponse(message.To))
}
