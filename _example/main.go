package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/marcoschwartz/lungo/pkg/lungo"
)

//go:embed app/*
var appFS embed.FS

//go:embed static/*
var staticFS embed.FS

func main() {
	dev := os.Getenv("LUNGO_DEV") == "1"
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	opts := lungo.Options{Dev: dev}

	if dev {
		opts.AppDir = "./app"
		opts.StaticDir = "./static"
	} else {
		appSub, _ := fs.Sub(appFS, "app")
		staticSub, _ := fs.Sub(staticFS, "static")
		opts.AppFS = appSub
		opts.StaticFS = staticSub
	}

	app := lungo.New(opts)

	// ── Middleware ──────────────────────────────────────────────────

	app.Use(lungo.CORS(lungo.CORSOptions{
		AllowOrigins: []string{"*"},
	}))

	app.Use(lungo.Redirects([]lungo.RedirectRule{
		{From: "/home", To: "/", Code: 301},
	}))

	// ── API Routes ─────────────────────────────────────────────────

	app.API("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":   "Hello from Go!",
			"timestamp": time.Now().Format(time.RFC3339),
			"features": []string{
				"Server-Side Rendering",
				"Streaming SSR",
				"File-Based Routing",
				"Middleware",
				"Server Actions",
				"Static Site Generation",
				"SEO Metadata",
				"Go API Routes",
				"Single Binary Deploy",
			},
		})
	})

	app.API("/api/posts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		posts := []map[string]string{
			{"id": "1", "title": "Getting Started with Lungo", "excerpt": "Learn how to build apps with Lungo"},
			{"id": "2", "title": "SSR in Go", "excerpt": "Server-side rendering without Node.js"},
			{"id": "3", "title": "No Build Step", "excerpt": "Why tagged template literals are the future"},
		}
		json.NewEncoder(w).Encode(posts)
	})

	// ── Blog API ──────────────────────────────────────────────────

	blogPosts := map[string]map[string]string{
		"getting-started": {
			"slug":    "getting-started",
			"title":   "Getting Started with Our Framework",
			"date":    "2026-03-10",
			"author":  "Marco",
			"content": "Welcome to our Go-powered framework! This post walks you through setting up your first project.\n\nUnlike Next.js, there's no node_modules to install. Just write your components in .js or .jsx files, register your API routes in Go, and run a single binary.\n\nThe framework supports file-based routing, SSR, streaming, middleware, server actions, and static site generation — all powered by Go.",
		},
		"jsx-without-node": {
			"slug":    "jsx-without-node",
			"title":   "JSX Without Node.js",
			"date":    "2026-03-12",
			"author":  "Marco",
			"content": "Our built-in JSX transpiler is written in pure Go — about 300 lines of code.\n\nIt converts standard JSX syntax into h() function calls on the fly. No Babel, no Webpack, no esbuild. The Go server reads your .jsx file, transpiles it in microseconds, and serves plain JavaScript to the browser.\n\nThis means you get the familiar React-like syntax with zero JavaScript toolchain dependencies.",
		},
		"sse-realtime": {
			"slug":    "sse-realtime",
			"title":   "Real-Time Data with Server-Sent Events",
			"date":    "2026-03-14",
			"author":  "Marco",
			"content": "Go's goroutines make real-time streaming trivial. Just write to the http.ResponseWriter and call Flush().\n\nThe framework doesn't need WebSocket libraries or Socket.IO — plain SSE works perfectly for server-to-client streaming. The Live Dashboard demo shows gauges, sparklines, and event logs updating every second.\n\nCombined with our virtual DOM, the UI re-renders efficiently on each event without any jank.",
		},
	}

	app.API("/api/blog", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var list []map[string]string
		for _, p := range blogPosts {
			list = append(list, map[string]string{
				"slug":   p["slug"],
				"title":  p["title"],
				"date":   p["date"],
				"author": p["author"],
			})
		}
		json.NewEncoder(w).Encode(list)
	})

	app.API("/api/blog/post", func(w http.ResponseWriter, r *http.Request) {
		slug := r.URL.Query().Get("slug")
		post, ok := blogPosts[slug]
		if !ok {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(post)
	})

	// ── SSE (Server-Sent Events) ───────────────────────────────────

	app.API("/api/sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		// Send initial event
		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case t := <-ticker.C:
				// Simulated metrics
				cpu := 20 + (t.UnixMilli()%60)*1
				mem := 40 + (t.UnixMilli()%30)*1
				rps := 800 + (t.UnixMilli() % 500)
				latency := 2 + (t.UnixMilli() % 15)

				data := fmt.Sprintf(`{"time":"%s","cpu":%d,"memory":%d,"rps":%d,"latency":%d}`,
					t.Format("15:04:05"), cpu%100, mem%100, rps, latency)
				fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	})

	// ── Server Actions ─────────────────────────────────────────────

	app.Action("contact", func(w http.ResponseWriter, r *http.Request) lungo.ActionResult {
		name := r.FormValue("name")
		email := r.FormValue("email")
		message := r.FormValue("message")

		if email == "" {
			return lungo.ActionResult{Error: "Email is required"}
		}

		// Process the form (in real app: save to DB, send email, etc.)
		fmt.Printf("[Action] Contact form: name=%s email=%s message=%s\n", name, email, message)

		return lungo.ActionResult{
			Redirect: "/contact?success=1",
			Data:     map[string]string{"status": "sent"},
		}
	})

	// ── Command ────────────────────────────────────────────────────

	switch cmd {
	case "build":
		// Static site generation
		outDir := "./out"
		if len(os.Args) > 2 {
			outDir = os.Args[2]
		}
		if err := app.Build(lungo.BuildOptions{OutputDir: outDir}); err != nil {
			log.Fatal(err)
		}
	default:
		port := os.Getenv("PORT")
		if port == "" {
			port = "3000"
		}
		log.Fatal(app.ListenAndServe(":" + port))
	}
}
