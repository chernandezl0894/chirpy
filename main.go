package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset to 0"))
}

func main() {
	port := "8080"
	apiConfig := apiConfig{
		fileserverHits: atomic.Int32{},
	}
	fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux := http.NewServeMux()
	mux.Handle("/app/", apiConfig.middlewareMetricsInc(fileServer))
	mux.HandleFunc("GET /metrics", apiConfig.handlerMetrics)
	mux.HandleFunc("POST /reset", apiConfig.handlerReset)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(http.StatusText(http.StatusOK)))
	})

	s := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Println(fmt.Sprintf("Running server at http://127.0.0.1:%s/", port))
	log.Fatal(s.ListenAndServe())
}
