package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/andfenastari/chatsim/core"
	"github.com/google/uuid"
)

type Server struct {
	http.ServeMux

	Core     *core.Core
	Client   http.Client
	Webhooks sync.Map
}

type Webhook struct {
	User string
	URL  *url.URL
}

func NewServer(core *core.Core) *Server {
	server := new(Server)
	server.Core = core

	server.HandleFunc("POST /{user}/messages", server.handleCreateMessage)
	server.HandleFunc("POST /{user}/webhooks", server.handleCreateWebhook)
	server.HandleFunc("DELETE /{user}/webhooks/{id}", server.handleDeleteWebhook)

	return server
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")

	var msg *core.Message
	if s.decodeJSON(w, r, &msg) {
		return
	}

	msg.From = user

	s.Core.Mux.Lock()
	defer s.Core.Mux.Unlock()

	chat := s.Core.GetOrCreateChat([]string{msg.From, msg.To})
	s.Core.AddMessage(chat, msg)
}

type CreateWebhookRequest struct {
	URL string `json:"url"`
}

type CreateWebhookResponse struct {
	Id string `json:"id"`
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")

	var req CreateWebhookRequest
	if s.decodeJSON(w, r, &req) {
		return
	}

	url, err := url.Parse(req.URL)
	if err != nil {
		log.Print("Failed to decode url: %v", err)
		http.Error(w, "Invalid request url", http.StatusBadRequest)
		return
	}

	id := uuid.NewString()
	s.Webhooks.Store(id, Webhook{
		URL:  url,
		User: user,
	})

	s.encodeJSON(w, CreateWebhookResponse{Id: id})
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.Webhooks.Delete(id)
}

type jsonObject = map[string]interface{}
type jsonArray = []interface{}

func (s *Server) notifyWebhooks() {
	events := s.Core.AddListener()

	for event := range events {
		s.Webhooks.Range(func(key, val any) bool {
			id := key.(string)
			webhook := val.(Webhook)

			if webhook.User != event.To {
				return true
			}

			body := jsonObject{
				"object": "whatsapp_business_account",
				"entry": jsonArray{
					jsonObject{
						"id": event.To,
						"changes": jsonArray{
							jsonObject{
								"field": "messages",
								"value": jsonObject{
									"messaging_product": "whatsapp",
									"metadata": jsonObject{
										"display_phone_number": event.From,
										"phone_number_id":      event.From,
									},
									"messages": jsonArray{event},
								},
							},
						},
					},
				},
			}

			bodyBytes, _ := json.Marshal(body)
			bodyReader := bytes.NewReader(bodyBytes)
			_, err := s.Client.Post(webhook.URL.String(), "text/json", bodyReader)
			if err != nil {
				log.Print("Failed to send webhook %s to %s: %v", id, webhook.User, err)
			}

			return true
		})
	}
}

func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, val any) (failed bool) {
	err := json.NewDecoder(r.Body).Decode(val)
	if err != nil {
		log.Print("Failed to decode body %T: %v", val, err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return true
	}

	return false
}

func (s *Server) encodeJSON(w http.ResponseWriter, val any) (failed bool) {
	err := json.NewEncoder(w).Encode(val)
	if err != nil {
		log.Print("Failed to encode response %T: %v", val, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return true
	}

	return false
}
