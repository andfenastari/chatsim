package core

import (
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
)

type Core struct {
	ctx context.Context

	addListener    chan chan *Message
	removeListener chan chan *Message
	events         chan *Message

	Mux   sync.Mutex
	Users []string
	Chats []*Chat `json:"chats"`
	Media []*Media
}

type Chat struct {
	Members  []string   `json:"members"`
	Messages []*Message `json:"messages"`
}

type Message struct {
	From string       `json:"from"`
	To   string       `json:"to"`
	Type string       `json:"type"`
	Text *TextMessage `json:"text,omitempty"`
}

type TextMessage struct {
	Body string `json:"body"`
}

type Media struct {
	Id   string `json:"id"`
	User string `json:"user"`
	Type string `json:"type"`
	Data []byte `json:"data"`
}

func NewCore(ctx context.Context) *Core {
	core := new(Core)
	core.events = make(chan *Message, 10)
	core.addListener = make(chan chan *Message, 10)
	core.removeListener = make(chan chan *Message, 10)
	core.ctx = ctx

	go core.notifyListeners()

	return core
}

func (c *Core) GetOrCreateChat(members []string) *Chat {
	for _, chat := range c.Chats {
		found := true
		for _, member := range members {
			if !slices.Contains(chat.Members, member) {
				found = false
				break
			}
		}

		if found {
			return chat
		}
	}

	chat := &Chat{Members: members}
	c.Chats = append(c.Chats, chat)

	return chat
}

func (c *Core) AddMessage(chat *Chat, msg *Message) {
	chat.Messages = append(chat.Messages, msg)
	c.events <- msg
}

func (c *Core) AddMedia(user, typ string, data []byte) string {
	id := uuid.NewString()
	m := &Media{
		Id:   id,
		User: user,
		Type: typ,
		Data: data,
	}
	c.Media = append(c.Media, m)

	return id
}

func (c *Core) GetMedia(id string) *Media {
	for _, media := range c.Media {
		if media.Id == id {
			return media
		}
	}

	return nil
}

func (c *Core) AddListener() chan *Message {
	l := make(chan *Message, 10)
	c.addListener <- l

	return l
}

func (c *Core) RemoveListener(l chan *Message) {
	c.removeListener <- l
}

func (c *Core) notifyListeners() {
	listeners := map[chan *Message]bool{}
	done := c.ctx.Done()

	defer func() {
		for listener := range listeners {
			close(listener)
		}
	}()

	for {
		select {
		case msg := <-c.events:
			for listener := range listeners {
				listener <- msg
			}
		case listener := <-c.addListener:
			listeners[listener] = true
		case listener := <-c.removeListener:
			delete(listeners, listener)
		case <-done:
			return
		}
	}
}
