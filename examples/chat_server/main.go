package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/andfenastari/chatsim/core"
	"github.com/andfenastari/chatsim/shell/api"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	chatsim = flag.String("chatsim", "http://localhost:8000", "Chatsim server url. Default: 'https://localhost:8000/'")
	user    = flag.String("user", "", "User that is chatting.")
	peer    = flag.String("peer", "", "Peer to chat with.")
	port    = flag.String("port", "", "Port to listen for webhooks.")
)

var (
	closeServer  = make(chan bool)
	serverClosed = make(chan bool)
)

type Model struct {
	mux      sync.Mutex
	ctx      context.Context
	activity chan bool

	Messages []*core.Message
	Current  string
}

type activityMsg struct{}

type jsonObject = map[string]interface{}
type jsonArray = []interface{}

func main() {
	flag.Parse()

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func initialModel() *Model {
	m := new(Model)
	m.activity = make(chan bool, 10)

	return m
}

func (m *Model) Init() tea.Cmd {
	go m.listen()
	return m.activityCmd
}

func (m *Model) activityCmd() tea.Msg {
	<-m.activity
	return activityMsg{}
}

func (m *Model) quitCmd() tea.Msg {
	closeServer <- true
	<-serverClosed

	return tea.QuitMsg{}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyRunes {
			m.Current += msg.String()
		} else if msg.Type == tea.KeyEnter {
			go m.sendMessage(m.Current)
			m.Current = ""
		} else if msg.Type == tea.KeyCtrlC {
			return m, m.quitCmd
		}

		return m, nil
	case activityMsg:
		return m, m.activityCmd
	}

	return m, nil
}

func (m *Model) View() string {
	lines := []string{
		"Chat with " + *peer,
		"Press Ctrl+C to quit",
	}

	for _, msg := range m.Messages {
		lines = append(lines, fmt.Sprintf("%s: %s", msg.From, msg.Text.Body))
	}
	lines = append(lines, "> "+m.Current)

	return strings.Join(lines, "\n")
}

func (m *Model) listen() {
	client := &http.Client{}

	wreq := api.CreateWebhookRequest{
		URL: "http://localhost:" + *port,
	}
	rres, err := client.Post(fmt.Sprintf("%s/%s/webhooks", *chatsim, *user), "text/json", jsonReader(wreq))
	if err != nil {
		panic(err)
	}

	if rres.StatusCode != http.StatusOK {
		k, _ := io.ReadAll(rres.Body)
		log.Printf("Create webhook error (%d): %s", rres.StatusCode, k)
	}
	var wres api.CreateWebhookResponse
	jsonDecode(rres.Body, &wres)

	handler := &http.ServeMux{}

	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var req jsonObject
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			log.Fatalf("Couldn't decode webhook: %v", err)
		}

		entries := req["entry"].(jsonArray)
		entry := entries[0].(jsonObject)
		changes := entry["changes"].(jsonArray)
		change := changes[0].(jsonObject)
		value := change["value"].(jsonObject)
		messages := value["messages"].(jsonArray)
		message := messages[0].(jsonObject)
		text := message["text"].(jsonObject)
		body := text["body"].(string)
		metadata := value["metadata"].(jsonObject)
		phone_number := metadata["phone_number_id"].(string)

		if phone_number != *peer {
			return
		}

		m.mux.Lock()
		m.Messages = append(m.Messages, &core.Message{
			From: *peer,
			To:   *user,
			Type: "text",
			Text: &core.TextMessage{
				Body: body,
			},
		})
		m.mux.Unlock()
		m.activity <- true
	})

	srv := http.Server{Addr: fmt.Sprintf(":%s", *port), Handler: handler}
	go func() {
		<-closeServer
		srv.Close()
		req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/%s/webhooks/%s", *chatsim, *user, wres.Id), nil)
		client.Do(req)
		serverClosed <- true
	}()

	srv.ListenAndServe()
}

func (m *Model) sendMessage(text string) {
	client := http.Client{}
	msg := &core.Message{
		To:   *peer,
		From: *user,
		Type: "text",
		Text: &core.TextMessage{Body: text},
	}

	_, err := client.Post(fmt.Sprintf("%s/%s/messages", *chatsim, *user), *chatsim, jsonReader(msg))
	if err != nil {
		log.Printf("Error sending message: %v", err)
	}

	m.mux.Lock()
	m.Messages = append(m.Messages, msg)
	m.mux.Unlock()
	m.activity <- true
}

func jsonReader(val any) io.Reader {
	// log.Printf("Reading json: %T", val)
	b, err := json.Marshal(val)
	if err != nil {
		log.Fatal(err)
	}
	return bytes.NewReader(b)
}

func jsonDecode(r io.Reader, val any) {
	// log.Printf("Decoding json: %T", val)
	err := json.NewDecoder(r).Decode(val)
	if err != nil {
		log.Fatal(err)
	}
}
