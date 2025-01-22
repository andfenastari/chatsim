package core

import (
	"context"
	"crypto/sha256"
	"log"
	"slices"
	"sync"

	"github.com/google/uuid"
)

var (
	MediaTypes = map[string]string{
		"MP3":             "audio/mpeg",
		"JPEG":            "image/jpeg",
		"PNG":             "image/png",
		"Microsoft Excel": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}
)

type Core struct {
	sync.RWMutex

	ctx context.Context

	addListener    chan chan *Message
	removeListener chan chan *Message
	events         chan *Message

	Users []string
	Chats []*Chat `json:"chats"`
	Media []*Media
}

type Chat struct {
	Members  [2]string  `json:"members"`
	Messages []*Message `json:"messages"`
}

type Message struct {
	From string       `json:"from"`
	To   string       `json:"to"`
	Type string       `json:"type"`
	Text *TextMessage `json:"text,omitempty"`
	Data interface{}
}

type TextMessage struct {
	Body string `json:"body"`
}

type Media struct {
	Id   string `json:"id"`
	User string `json:"user"`
	Type string `json:"type"`
	Data []byte `json:"data"`
	Hash []byte `json:"hash"`
}

func (m *Media) ContentType() string {
	return MediaTypes[m.Type]
}

func (c *Chat) Peer(user string) string {
	if c.Members[0] == user {
		return c.Members[1]
	} else {
		return c.Members[0]
	}
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

func (c *Core) GetOrCreateChat(members [2]string) *Chat {
out:
	for _, chat := range c.Chats {
		for _, member := range members {
			if !slices.Contains(chat.Members[:], member) {
				log.Printf("Chat does not cointain members %v: %+v", members, chat)
				continue out
			}
		}
		log.Printf("Chat contains members %v: %+v", members, chat)
		return chat
	}

	chat := &Chat{Members: members}
	c.Chats = append(c.Chats, chat)
	log.Printf("Chat created: %+v", chat)

	return chat
}

func (c *Core) AddMessage(chat *Chat, msg *Message) {
	chat.Messages = append(chat.Messages, msg)
	c.events <- msg
}

func (c *Core) AddMedia(user, typ string, data []byte) string {
	id := uuid.NewString()
	hash := sha256.Sum256(data)

	m := &Media{
		Id:   id,
		User: user,
		Type: typ,
		Data: data,
		Hash: hash[:],
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
