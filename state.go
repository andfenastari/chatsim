package main

import (
	"sync"
)

type State struct {
	mux   sync.Mutex
	Chats []*Chat `json:"chats"`
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
}
