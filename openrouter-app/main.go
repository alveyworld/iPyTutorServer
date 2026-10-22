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

// openRouterURL is a var so tests can point it at a fake upstream.
var openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

const (
	maxBodySize = 1 << 20 // 1 MiB

	openRouterResponseTimeout = 120 * time.Second
)

var openRouterClient = &http.Client{
	Timeout: openRouterResponseTimeout,
}

func main() {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable is required")
	}

	privateKeyPEM, err := loadPrivateKeyPEM()
	if err != nil {
		log.Fatal(err)
	}

	auth, err := newAuthService(privateKeyPEM)
	if err != nil {
		log.Fatalf("auth setup failed: %v", err)
	}

	agentStates := newAgentStateStore()
	turnStore := newTurnStore()

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
	mux.HandleFunc("POST /api/session/bootstrap", auth.requireSession(handleBootstrap(agentStates, apiKey)))
	mux.HandleFunc("GET /api/session/state", auth.requireSession(handleSessionState(agentStates)))
	mux.HandleFunc("POST /api/chat", auth.requireSession(handleChat(apiKey, agentStates, turnStore)))
	mux.Handle("GET /{$}", http.FileServer(http.FS(static)))
	mux.Handle("GET /style.css", contentHandler(static, "style.css", "text/css; charset=utf-8"))
	mux.Handle("GET /app.js", contentHandler(static, "app.js", "application/javascript"))
	mux.Handle("GET /marked.min.js", contentHandler(static, "marked.min.js", "application/javascript"))
	mux.Handle("GET /dompurify.min.js", contentHandler(static, "dompurify.min.js", "application/javascript"))

	server := &http.Server{
		Addr:         addr,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 3 * openRouterResponseTimeout, // allow up to 3 OpenRouter turns per chat request
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("listening on %s", addr)
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
