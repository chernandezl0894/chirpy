package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chernandezl0894/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	platform       string
	db             *database.Queries
}

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	if cfg.platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(http.StatusText(http.StatusForbidden)))
		return
	}
	cfg.fileserverHits.Store(0)
	err := cfg.db.DeleteAllUsers(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(http.StatusText(http.StatusInternalServerError)))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func (cfg *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	type errorResponse struct {
		Error string `json:"error"`
	}
	w.Header().Set("Content-Type", "application/json")

	rows, err := cfg.db.GetChirps(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		message, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(message)
		return
	}
	chirps := []Chirp{}

	for _, row := range rows {
		chirps = append(chirps, Chirp{
			ID:        row.ID,
			Body:      row.Body,
			UserID:    row.UserID,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	dat, err := json.Marshal(chirps)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		message, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(message)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

func (cfg *apiConfig) handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	type errorResponse struct {
		Error string `json:"error"`
	}
	w.Header().Set("Content-Type", "application/json")

	userId := r.PathValue("chirpID")
	parsedUUID, err := uuid.Parse(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		message, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(message)
		return
	}
	chirp, err := cfg.db.GetChirp(r.Context(), parsedUUID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		message, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusNotFound)})
		w.Write(message)
		return
	}
	dat, err := json.Marshal(Chirp{
		ID:        chirp.ID,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		message, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(message)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

func (cfg *apiConfig) handlerCreateChirp(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}
	type errorResponse struct {
		Error string `json:"error"`
	}

	w.Header().Set("Content-Type", "application/json")
	decoder := json.NewDecoder(r.Body)
	params := request{}
	err := decoder.Decode(&params)
	badWords := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}

	var dat []byte

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusBadRequest)})
		w.Write(dat)
		return
	}

	if len(params.Body) > 140 {
		w.WriteHeader(http.StatusBadRequest)
		dat, _ = json.Marshal(errorResponse{Error: "Chirp is too long"})
		w.Write(dat)
		return
	}
	createChirpParams := database.CreateChirpParams{
		Body:   getCleanedMessage(params.Body, badWords),
		UserID: params.UserID,
	}
	chirp, err := cfg.db.CreateChirp(r.Context(), createChirpParams)
	dat, err = json.Marshal(Chirp{
		ID:        chirp.ID,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}
	w.WriteHeader(http.StatusCreated)
	w.Write(dat)
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email string `json:"email"`
	}
	type errorResponse struct {
		Error string `json:"error"`
	}

	w.Header().Set("Content-Type", "application/json")
	decoder := json.NewDecoder(r.Body)
	params := request{}
	err := decoder.Decode(&params)

	var dat []byte

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusBadRequest)})
		w.Write(dat)
		return
	}

	user, err := cfg.db.CreateUser(r.Context(), params.Email)
	dat, err = json.Marshal(User{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}
	w.WriteHeader(http.StatusCreated)
	w.Write(dat)
}

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Something went wrong")
		os.Exit(1)
	}
	dbQueries := database.New(db)

	port := "8080"
	apiConfig := apiConfig{
		fileserverHits: atomic.Int32{},
		platform:       platform,
		db:             dbQueries,
	}
	fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux := http.NewServeMux()
	mux.Handle("/app/", apiConfig.middlewareMetricsInc(fileServer))

	mux.HandleFunc("GET /admin/metrics", apiConfig.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiConfig.handlerReset)

	mux.HandleFunc("POST /api/users", apiConfig.handlerCreateUser)

	mux.HandleFunc("GET /api/chirps", apiConfig.handlerGetChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiConfig.handlerGetChirp)
	mux.HandleFunc("POST /api/chirps", apiConfig.handlerCreateChirp)

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
