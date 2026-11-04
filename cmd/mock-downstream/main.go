package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/logger"
)

func main() {
	lg, err := logger.NewLogger("local")
	if err != nil {
		os.Exit(1)
	}
	name := os.Getenv("NAME")
	if name == "" {
		name = "mock-downstream"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&env)
		lg.Info("received event", map[string]any{"name": name, "id": env.ID, "type": env.Type, "path": r.URL.Path})
		w.WriteHeader(http.StatusOK)
	})

	lg.Info("listening", map[string]any{"name": name, "port": port})
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		lg.Error("serve failed", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}
