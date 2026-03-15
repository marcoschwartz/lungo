package lungo

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

// HMR handles hot module replacement via WebSocket + file watching.
type HMR struct {
	appDir   string
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
	upgrader websocket.Upgrader
}

// NewHMR creates a new HMR instance that watches the app directory.
func NewHMR(appDir string) *HMR {
	h := &HMR{
		appDir:  appDir,
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	go h.watch()
	return h
}

// ServeWS handles WebSocket upgrade for HMR connections.
func (h *HMR) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[HMR] upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	log.Println("[HMR] client connected")

	// Keep connection alive, remove on close
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
			break
		}
	}
}

func (h *HMR) broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

func (h *HMR) watch() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[HMR] watcher error: %v", err)
		return
	}
	defer watcher.Close()

	// Walk the app directory and watch all directories
	filepath.WalkDir(h.appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			watcher.Add(path)
		}
		return nil
	})

	log.Printf("[HMR] watching %s for changes", h.appDir)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				log.Printf("[HMR] file changed: %s", event.Name)
				h.broadcast([]byte(`{"type":"reload","path":"` + event.Name + `"}`))
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[HMR] error: %v", err)
		}
	}
}
