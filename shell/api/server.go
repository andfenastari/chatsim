package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/andfenastari/chatsim/core"
	"github.com/google/uuid"
)

type Handler struct {
	http.ServeMux

	Core     *core.Core
	Client   http.Client
	Webhooks sync.Map
}

type Webhook struct {
	User string
	URL  *url.URL
}

func NewHandler(core *core.Core) *Handler {
	handler := new(Handler)
	handler.Core = core

	handler.HandleFunc("POST /{user}/messages", handler.handleCreateMessage)
	handler.HandleFunc("GET /{user}/messages", handler.handleListMessages)
	handler.HandleFunc("POST /{user}/webhooks", handler.handleCreateWebhook)
	handler.HandleFunc("DELETE /{user}/webhooks/{id}", handler.handleDeleteWebhook)
	handler.HandleFunc("POST /{user}/media", handler.handleCreateMedia)
	handler.HandleFunc("GET /{media}", handler.handleViewMedia)
	handler.HandleFunc("GET /{media}/download", handler.handleDownloadMedia)

	go handler.notifyWebhooks()

	return handler
}

func (s *Handler) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")

	var msg *core.Message
	if s.decodeJSON(w, r, &msg) {
		return
	}

	msg.From = user

	log.Printf("Received message: %+v", msg)

	s.Core.Lock()
	defer s.Core.Unlock()

	chat := s.Core.GetOrCreateChat([2]string{msg.From, msg.To})
	s.Core.AddMessage(chat, msg)
}

func (s *Handler) handleListMessages(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	peer := r.FormValue("peer")

	if peer == "" {
		http.Error(w, "Missing peer query parameter", http.StatusBadRequest)
	}

	s.Core.Lock()
	chat := s.Core.GetOrCreateChat([2]string{user, peer})
	s.Core.Unlock()

	log.Print(chat.Messages)

	if s.encodeJSON(w, chat.Messages) {
		return
	}
}

type CreateWebhookRequest struct {
	URL string `json:"url"`
}

type CreateWebhookResponse struct {
	Id string `json:"id"`
}

func (s *Handler) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")

	var req CreateWebhookRequest
	if s.decodeJSON(w, r, &req) {
		return
	}

	log.Printf("Received webhook: %+v", req)

	url, err := url.Parse(req.URL)
	if err != nil {
		log.Print("Failed to decode url: %v", err)
		http.Error(w, "Invalid request url", http.StatusBadRequest)
		return
	}

	id := s.RegisterWebhook(user, url)

	s.encodeJSON(w, CreateWebhookResponse{Id: id})
}

func (s *Handler) RegisterWebhook(user string, url *url.URL) string {
	id := uuid.NewString()
	s.Webhooks.Store(id, Webhook{
		URL:  url,
		User: user,
	})

	return id
}

func (s *Handler) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	log.Printf("Deleted webhook: %+v", id)

	s.Webhooks.Delete(id)
}

type CreateMediaResponse struct {
	Id string `json:"id"`
}

func (s *Handler) handleCreateMedia(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")

	r.ParseMultipartForm(1_000)
	typ := r.MultipartForm.Value["type"][0]
	header := r.MultipartForm.File["file"][0]
	file, err := header.Open()
	if err != nil {
		log.Fatalf("Failed to open media file: %v", err)
		http.Error(w, "Internal handler error", http.StatusInternalServerError)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("Failed to read media file: %v", err)
		http.Error(w, "Internal handler error", http.StatusInternalServerError)
	}

	s.Core.Lock()
	id := s.Core.AddMedia(user, typ, data)
	s.Core.Unlock()

	s.encodeJSON(w, CreateMediaResponse{Id: id})
}

type ViewMediaResponse struct {
	MessagingProduct string `json:"messaging_product"`
	URL              string `json:"url"`
	Sha256           []byte `json:"sha256"`
	MimeType         string `json:"mime_type"`
	FileSize         int    `json:"file_size"`
	Id               string `json:"id"`
}

func (s *Handler) handleViewMedia(w http.ResponseWriter, r *http.Request) {
	mediaId := r.PathValue("media")

	s.Core.RLock()
	media := s.Core.GetMedia(mediaId)
	s.Core.RUnlock()

	if media == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	baseUrl := r.URL
	log.Print(baseUrl)

	s.encodeJSON(w, ViewMediaResponse{
		MessagingProduct: "whatsapp",
		URL:              fmt.Sprintf("http://localhost:8000/%s/download", mediaId), // TODO: Un-fake url
		Sha256:           media.Hash,
		MimeType:         media.ContentType(),
		FileSize:         len(media.Data),
		Id:               media.Id,
	})
}

func (s *Handler) handleDownloadMedia(w http.ResponseWriter, r *http.Request) {
	mediaId := r.PathValue("media")

	s.Core.RLock()
	media := s.Core.GetMedia(mediaId)
	s.Core.RUnlock()

	w.Header().Set("Content-Type", media.ContentType())
	w.Write(media.Data)
}

type jsonObject = map[string]interface{}
type jsonArray = []interface{}

func (s *Handler) notifyWebhooks() {
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
			_, err := s.Client.Post(webhook.URL.String(), "application/json", bodyReader)
			if err != nil {
				log.Printf("Failed to send webhook %s to %s: %v", id, webhook.User, err)
			}

			log.Printf("Sent webhook: %+v", body)

			return true
		})
	}
}

func (s *Handler) decodeJSON(w http.ResponseWriter, r *http.Request, val any) (failed bool) {
	log.Printf("Decoding %T", val)
	err := json.NewDecoder(r.Body).Decode(val)
	if err != nil {
		log.Print("Failed to decode body %T: %v", val, err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return true
	}

	return false
}

func (s *Handler) encodeJSON(w http.ResponseWriter, val any) (failed bool) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(val)
	if err != nil {
		log.Print("Failed to encode response %T: %v", val, err)
		http.Error(w, "Internal handler error", http.StatusInternalServerError)
		return true
	}

	return false
}
