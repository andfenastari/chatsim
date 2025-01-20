package main

import (
	"embed"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"

	//	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/andfenastari/templatemap"
)

var (
	port         = flag.Int("port", 8000, "Shttptest#NewRequesterver port. Defaults to '8000'.")
	webhook      = flag.String("webhook", "", "URL to send message webhooks. Must supply a value.")
	mediaDir     = flag.String("media", "", "Media storage directory. Defaults to a random temporary file.")
	snapshotPath = flag.String("snapshot", "", "Path of the snapshot to load.")
	agentPhone   = flag.String("agent-phone", "+00", "Phone number to use for the agent. Defaults to '+00'")
)

//go:embed templates/*
var templateFs embed.FS

type Server struct {
	http.ServeMux
	State *State
	Tmap  templatemap.Map
}

func main() {
	flag.CommandLine.Usage = usage
	flag.Parse()
	// if *webhook == "" {
	// 	fmt.Fprintf(os.Stderr, "error: Must supply a webhook url.")
	// 	usage()
	// 	os.Exit(1)
	// }

	server := newServer(NewState())

	log.Fatal(http.ListenAndServe(":8000", server))
}

func usage() {
	fmt.Fprintf(os.Stderr, "USAGE: %s [flag]...\nAvailable flags:\n", os.Args[0])
	flag.PrintDefaults()
}

func newServer(state *State) *Server {
	// dir, _ := fs.Sub(templateFs, "templates")
	// tmap, err := templatemap.ParseFS(dir)
	tmap, err := templatemap.ParseDir("templates")
	if err != nil {
		log.Fatal("Failed to load templates.")
	}

	server := &Server{State: state, Tmap: tmap}
	server.HandleFunc("POST /v17.0/{sender}/messages", server.handleCreateMessage)
	server.HandleFunc("GET /web/", server.handleIndex)
	server.HandleFunc("GET /web/{user}", server.handleChat)
	server.HandleFunc("POST /web/{user}", server.handleMessage)
	server.HandleFunc("GET /web/{user}/events", server.handleEvents)
	server.HandleFunc("GET /web/create", server.handleCreateForm)
	server.HandleFunc("POST /web/create", server.handleCreate)
	server.Handle("/static/", http.FileServer(http.Dir(".")))
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

func (s *Server) handleIndex(w http.ResponseWriter, req *http.Request) {
	if len(s.State.Chats) == 0 {
		http.Redirect(w, req, "/web/create", http.StatusFound)
		return
	} else {
		user := s.State.Chats[0].User
		http.Redirect(w, req, "/web/"+user, http.StatusFound)
		return
	}
}

func (s *Server) handleCreateForm(w http.ResponseWriter, req *http.Request) {
	s.responseTemplate(w, "create.tmpl", nil)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	user := r.FormValue("user")
	if user == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	s.State.mux.Lock()
	s.State.GetOrCreateChat(user)
	s.State.mux.Unlock()

	http.Redirect(w, r, "/web/"+user, http.StatusFound)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	s.State.mux.Lock()
	chat := s.State.GetOrCreateChat(user)
	s.State.mux.Unlock()

	for _, msg := range chat.Messages {
		log.Print(msg.Text)
	}

	s.responseTemplate(w, "chat.tmpl", chat)
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	log.Print(r.FormValue("message"))
	msg := &Message{
		To:   *agentPhone,
		Type: "text",
		Text: &TextMessage{Body: r.FormValue("message")},
	}

	s.State.mux.Lock()
	s.State.AddMessage(user, msg)
	log.Print("message added")
	err := s.State.SendWebhook(user, msg)
	if err != nil {
		log.Printf("Webhook error: %v", err)
	}
	log.Print("webhook sent")
	s.State.mux.Unlock()

	s.responseTemplate(w, "message.tmpl", msg)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	log.Print("event connection: %s", user)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	done := r.Context().Done()
	events := s.State.AddListener(user)
	defer s.State.RemoveListener(user, events)

	for {
		select {
		case <-done:
			return
		case event := <-events:
			if event.User != user {
				continue
			}
			s.responseTemplate(w, "event.tmpl", event.Message)
		}
	}

	log.Print("event disconnected: %s", user)
}

func (s *Server) responseTemplate(w http.ResponseWriter, path string, data any) {
	var err error

	s.Tmap, err = templatemap.ParseDir("templates")
	if err != nil {
		log.Fatal("Failed to load templates.")
	}

	tmpl, ok := s.Tmap[path]
	if !ok {
		log.Fatalf("Failed to lookup template %s", path)
	}
	err = tmpl.Execute(w, struct {
		State *State
		Data  any
	}{
		State: s.State,
		Data:  data,
	})
	if err != nil {
		log.Fatalf("Failed to render template %s: %v", path, err)
	}
}
