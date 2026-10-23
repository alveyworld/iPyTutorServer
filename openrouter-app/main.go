package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

const maxBodySize = 1 << 20 // 1 MiB

func main() {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable is required")
	}
	llm := llmClient{apiKey: apiKey, url: defaultOpenRouterURL}
	if url := os.Getenv("OPENROUTER_URL"); url != "" {
		llm.url = url
	}

	privateKeyPEM, err := loadPrivateKeyPEM()
	if err != nil {
		log.Fatal(err)
	}
	auth, err := newAuthService(privateKeyPEM)
	if err != nil {
		log.Fatalf("auth setup failed: %v", err)
	}

	tutor := newTutor(llm)
	turns := newTurnStore()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	if listenAddr := os.Getenv("LISTEN_ADDR"); listenAddr != "" {
		addr = listenAddr + ":" + port
	}

	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /api/auth/challenge", auth.handleChallenge)
	mux.HandleFunc("POST /api/auth/verify", auth.handleVerify)
	mux.HandleFunc("POST /api/session/bootstrap", auth.requireSession(tutor.handleBootstrap))
	mux.HandleFunc("GET /api/session/state", auth.requireSession(tutor.handleState))
	mux.HandleFunc("POST /api/chat", auth.requireSession(tutor.handleChat(turns)))
	mux.Handle("GET /{$}", http.FileServer(http.FS(static)))
	mux.Handle("GET /style.css", contentHandler(static, "style.css", "text/css; charset=utf-8"))
	mux.Handle("GET /app.js", contentHandler(static, "app.js", "application/javascript"))
	mux.Handle("GET /marked.min.js", contentHandler(static, "marked.min.js", "application/javascript"))
	mux.Handle("GET /dompurify.min.js", contentHandler(static, "dompurify.min.js", "application/javascript"))
	mux.Handle("GET /parrot.svg", contentHandler(static, "parrot.svg", "image/svg+xml"))

	server := &http.Server{
		Addr:        addr,
		Handler:     loggingMiddleware(mux),
		ReadTimeout: 15 * time.Second,
		// A turn can wait for a prepared problem and then write one itself.
		WriteTimeout: 4 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("iPyTutor listening on %s (model %s)", addr, tutorModel())
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func contentHandler(static fs.FS, name, contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(static, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
