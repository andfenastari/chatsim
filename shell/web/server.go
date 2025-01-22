package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"

	"github.com/andfenastari/chatsim/core"
	"github.com/andfenastari/templatemap"
)

//go:embed templates static
var assets embed.FS

type Handler struct {
	http.ServeMux

	User         string
	Devel        bool
	SnapshotPath string

	Core *core.Core
	Tmap templatemap.Map
}

func arr(els ...any) []any {
	return els
}

func NewHandler(core *core.Core, user string, devel bool, snapshot string) *Handler {
	var err error

	var tmap templatemap.Map
	if !devel {
		var templateFS fs.FS

		templateFS, err = fs.Sub(assets, "templates")
		if err != nil {
			log.Fatal("Failed to load template dir", err)
		}

		parser := templatemap.Parser{
			FuncMap: template.FuncMap{"arr": arr},
		}

		tmap, err = parser.ParseFS(templateFS)
		if err != nil {
			log.Fatal("Failed to parse templates: ", err)
		}
		log.Print(tmap)
	}

	handler := &Handler{
		User:         user,
		Core:         core,
		Devel:        devel,
		SnapshotPath: snapshot,
		Tmap:         tmap,
	}

	handler.HandleFunc("GET /", handler.handleIndex)
	handler.HandleFunc("POST /snapshot/save", handler.handleSaveSnapshot)
	handler.HandleFunc("GET /snapshot/download", handler.handleDownloadSnapshot)
	handler.HandleFunc("GET /chat/{peer}", handler.handleChat)
	handler.HandleFunc("POST /chat/{peer}", handler.handleMessage)
	handler.HandleFunc("GET /chat/{peer}/events", handler.handleEvents)
	handler.HandleFunc("GET /media/{media}", handler.handleGetMedia)
	handler.HandleFunc("GET /chat/create", handler.handleCreateForm)
	handler.HandleFunc("POST /chat/create", handler.handleCreate)

	if !devel {
		handler.Handle("GET /static/", http.FileServer(http.FS(assets)))
	} else {
		handler.Handle("GET /static/", http.FileServer(http.Dir("./shell/web")))
	}

	return handler
}

func (s *Handler) handleIndex(w http.ResponseWriter, req *http.Request) {
	if len(s.Core.Chats) == 0 {
		http.Redirect(w, req, "/chat/create", http.StatusFound)
		return
	} else {
		peer := s.Core.Chats[0].Peer(s.User)

		http.Redirect(w, req, must(url.JoinPath("/chat/", peer)), http.StatusFound)
		return
	}
}

func (s *Handler) handleSaveSnapshot(w http.ResponseWriter, r *http.Request) {
	err := s.Core.SaveSnapshot(s.SnapshotPath)
	if err != nil {
		log.Print(err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *Handler) handleDownloadSnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	err := json.NewEncoder(w).Encode(s.Core.Snapshot)
	if err != nil {
		log.Printf("Failed to send snapshot: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *Handler) handleCreateForm(w http.ResponseWriter, req *http.Request) {
	s.responseTemplate(w, "create.tmpl", nil)
}

func (s *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	peer := r.FormValue("peer")
	if peer == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	s.Core.Lock()
	s.Core.GetOrCreateChat([2]string{s.User, peer})
	for _, chat := range s.Core.Chats {
		log.Print(chat)
	}
	s.Core.Unlock()

	http.Redirect(w, r, "/chat/"+peer, http.StatusFound)
}

func (s *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	peer := r.PathValue("peer")

	s.Core.Lock()
	chat := s.Core.GetOrCreateChat([2]string{s.User, peer})
	s.Core.Unlock()

	for _, msg := range chat.Messages {
		log.Print(msg.Text)
	}

	s.responseTemplate(w, "chat.tmpl", chat)
}

func (s *Handler) handleMessage(w http.ResponseWriter, r *http.Request) {
	peer := r.PathValue("peer")
	typ := r.FormValue("type")

	var msg *core.Message
	switch typ {
	case "text":
		text := r.FormValue("text")
		if text == "" {
			http.Error(w, "Must supply 'text' value.", http.StatusBadRequest)
		}
		msg = &core.Message{
			From: peer,
			To:   s.User,
			Type: "text",
			Text: &core.TextMessage{Body: text},
		}
	case "image":
		data, failed := readFile(w, r, "image")
		if failed {
			return
		}

		s.Core.Lock()
		id := s.Core.AddMedia(s.User, "PNG", data)
		s.Core.Unlock()

		caption := r.FormValue("caption")

		msg = &core.Message{
			From: peer,
			To:   s.User,
			Type: "image",
			Image: &core.ImageMessage{
				MediaId: id,
				Caption: caption,
			},
		}

	case "audio":
		data, failed := readFile(w, r, "audio")
		if failed {
			return
		}

		s.Core.Lock()
		id := s.Core.AddMedia(s.User, "MP3", data)
		s.Core.Unlock()

		msg = &core.Message{
			From: peer,
			To:   s.User,
			Type: "image",
			Audio: &core.AudioMessage{
				MediaId: id,
			},
		}
	default:
		http.Error(w, fmt.Sprintf("Unsupported message type '%s'.", typ), http.StatusBadRequest)
		return
	}

	s.Core.Lock()
	chat := s.Core.GetOrCreateChat([2]string{s.User, peer})
	s.Core.AddMessage(chat, msg)
	s.Core.Unlock()

	s.responseTemplate(w, "message.tmpl", msg)
}

func readFile(w http.ResponseWriter, r *http.Request, name string) (data []byte, failed bool) {

	file, _, err := r.FormFile(name)
	if err != nil {
		log.Printf("Failed to open media file: %v", err)
		http.Error(w, "Internal handler error", http.StatusInternalServerError)
		return nil, true
	}

	data, err = io.ReadAll(file)
	if err != nil {
		log.Printf("Failed to read media file: %v", err)
		http.Error(w, "Internal handler error", http.StatusInternalServerError)
		return nil, true
	}

	return data, false
}

func (s *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	peer := r.PathValue("peer")
	log.Printf("event connection: %s", peer)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	done := r.Context().Done()
	events := s.Core.AddListener()
	defer s.Core.RemoveListener(events)

out:
	for {
		select {
		case <-done:
			break out
		case msg := <-events:
			if msg.From != s.User || msg.To != peer {
				continue
			}
			s.responseSSE(w, "message.tmpl", msg)
		}
	}

	log.Print("event disconnected: %s", peer)
}

func (s *Handler) handleGetMedia(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("media")

	s.Core.RLock()
	media := s.Core.GetMedia(id)
	s.Core.RUnlock()

	w.Header().Set("Content-Type", media.ContentType())
	w.Write(media.Data)
}

type templateContext struct {
	State *Handler
	Data  any
}

func (s *Handler) template(path string) *template.Template {
	var err error
	var tmap templatemap.Map

	if !s.Devel {
		tmap = s.Tmap
	} else {
		parser := &templatemap.Parser{
			FuncMap: template.FuncMap{"arr": arr},
		}
		tmap, err = parser.ParseDir("./shell/web/templates")
		if err != nil {
			log.Fatal("Failed to load templates: ", err)
		}
	}

	tmpl, ok := tmap[path]
	if !ok {
		log.Fatalf("Failed to lookup template %s: %v", path, err)
	}

	return tmpl
}

func (s *Handler) responseTemplate(w http.ResponseWriter, path string, data any) {
	tmpl := s.template(path)

	err := tmpl.Execute(w, templateContext{
		State: s,
		Data:  data,
	})
	if err != nil {
		log.Fatalf("Failed to render template %s: %v", path, err)
	}
}

func (s *Handler) responseSSE(w http.ResponseWriter, path string, data any) {
	tmpl := s.template(path)

	buf := new(bytes.Buffer)
	err := tmpl.Execute(buf, templateContext{
		State: s,
		Data:  data,
	})
	if err != nil {
		log.Fatalf("Failed to render template %s: %v", path, err)
	}

	lines := bytes.Split(buf.Bytes(), []byte("\n"))
	for _, line := range lines {
		w.Write([]byte("data: "))
		w.Write(line)
		w.Write([]byte("\n"))
	}
	w.Write([]byte("\n\n"))
	w.(http.Flusher).Flush()
}

func must[T any](val T, err error) T {
	return val
}

func dumpDir(path string, d fs.DirEntry, err error) error {
	log.Print(path)
	return nil
}
