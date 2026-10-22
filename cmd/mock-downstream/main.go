package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
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
		log.Printf("[%s] received event id=%s type=%s path=%s", name, env.ID, env.Type, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("[%s] listening on :%s", name, port)
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
