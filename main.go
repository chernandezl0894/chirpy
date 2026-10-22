package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>
`, cfg.fileserverHits.Load())))
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
	mux.HandleFunc("GET /admin/metrics", apiConfig.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiConfig.handlerReset)
	mux.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Body string `json:"body"`
		}
		type returnVals struct {
			CleanedBody string `json:"cleaned_body"`
		}
		type errorResponse struct {
			Error string `json:"error"`
		}
		badWords := map[string]struct{}{
			"kerfuffle": {},
			"sharbert":  {},
			"fornax":    {},
		}
		w.Header().Set("Content-Type", "application/json")

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)

		var dat []byte

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			dat, _ := json.Marshal(errorResponse{Error: "Something went wrong"})
			w.Write(dat)
			return
		}

		if len(params.Body) > 140 {
			w.WriteHeader(http.StatusBadRequest)
			dat, _ = json.Marshal(errorResponse{Error: "Chirp is too long"})
			w.Write(dat)
			return
		}

		dat, err = json.Marshal(returnVals{
			CleanedBody: getCleanedMessage(params.Body, badWords),
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			dat, _ = json.Marshal(errorResponse{Error: "Something went wrong"})
			w.Write(dat)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(dat)
	})
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
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

func getCleanedMessage(msg string, badWords map[string]struct{}) string {
	words := strings.Split(string(msg), " ")
	for idx, word := range words {
		lowerWord := strings.ToLower(word)
		if _, ok := badWords[lowerWord]; ok {
			words[idx] = "****"
		}
	}
	return strings.Join(words, " ")
}
