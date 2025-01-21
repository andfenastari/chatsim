package main

import (
	"net/http"
	"sync"
)

type State struct {
	mux            sync.Mutex
	client         http.Client
	addListener    chan Listener
	removeListener chan Listener
	events         chan *Event
	AgentPhone     string
	Chats          []*Chat `json:"chats"`
}

type Event struct {
	User    string
	Message *Message
}

type Chat struct {
	User     string     `json:"user"`
	Messages []*Message `json:"messages"`
}

type Message struct {
	To   string       `json:"to"`
	Type string       `json:"type"`
	Text *TextMessage `json:"text,omitempty"`
}

type TextMessage struct {
	Body string `json:"body"`
}

type jsonObject = map[string]interface{}
type jsonList = []interface{}

type Listener struct {
	User   string
	Events chan *Event
}

func NewState() *State {
	events := make(chan *Event, 10)
	addListener := make(chan Listener, 10)
	removeListener := make(chan Listener, 10)

	go func() {
		listeners := make(map[Listener]bool)

		select {
		case listener := <-addListener:
			listeners[listener] = true
		case listener := <-removeListener:
			delete(listeners, listener)
		case event := <-events:
			for listener := range listeners {
				if event.User != listener.User {
					continue
				}
				listener.Events <- event
			}
		}
	}()

	return &State{
		AgentPhone:  *agentPhone,
		addListener: addListener,
		events:      events,
	}
}

func (s *State) GetOrCreateChat(user string) *Chat {
	for _, chat := range s.Chats {
		if chat.User == user {
			return chat
		}
	}

	// Chat doesn't exist yet.
	newChat := &Chat{User: user}
	s.Chats = append(s.Chats, newChat)
	return newChat
}

func (s *State) AddMessage(user string, msg *Message) {
	chat := s.GetOrCreateChat(user)
	chat.Messages = append(chat.Messages, msg)
	s.events <- &Event{User: user, Message: msg}
}

func (s *State) AddListener(user string) chan *Event {
	events := make(chan *Event, 10)
	s.addListener <- Listener{User: user, Events: events}
	return events
}

func (s *State) RemoveListener(user string, events chan *Event) {
	s.removeListener <- Listener{User: user, Events: events}
}

func (s *State) SendWebhook(user string, msg *Message) (err error) {
	return err
}
