// cmd/mock-downstream is a throwaway standalone binary used only by
// docker-compose to stand in for execution_service or definition_service's
// POST /internal/events endpoint, so `docker compose up` works without
// either real service checked out. It is not the same thing as
// test/mocks.Downstream, which is a Go test double used by unit and
// integration tests — this is a real, separately-run process.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
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
	mux.HandleFunc("/internal/events", func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&env)
		log.Printf("[%s] received event id=%s type=%s", name, env.ID, env.Type)
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("[%s] listening on :%s", name, port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
