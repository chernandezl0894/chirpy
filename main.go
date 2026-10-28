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

	auth "github.com/chernandezl0894/chirpy/internal/auth"
	"github.com/chernandezl0894/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	platform       string
	tokenString    string
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
		Body string `json:"body"`
	}
	type errorResponse struct {
		Error string `json:"error"`
	}

	w.Header().Set("Content-Type", "application/json")

	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}
	userID, err := auth.ValidateJWT(tokenString, cfg.tokenString)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := request{}
	err = decoder.Decode(&params)
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
		UserID: userID,
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

func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	type errorResponse struct {
		Error string `json:"error"`
	}
	w.Header().Set("Content-Type", "application/json")

	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}
	userID, err := auth.ValidateJWT(tokenString, cfg.tokenString)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusBadRequest)})
		w.Write(dat)
		return
	}

	chirp, err := cfg.db.GetChirp(r.Context(), chirpID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusNotFound)})
		w.Write(dat)
		return
	}

	if chirp.UserID != userID {
		w.WriteHeader(http.StatusForbidden)
		dat, _ := json.Marshal(errorResponse{Error: "You can only delete your own chirps"})
		w.Write(dat)
		return
	}

	err = cfg.db.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	type errorResponse struct {
		Error string `json:"error"`
	}

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	err = cfg.db.RevokeRefreshToken(r.Context(), database.RevokeRefreshTokenParams{
		RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		Token:     refreshToken,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Token string `json:"token"`
	}
	type errorResponse struct {
		Error string `json:"error"`
	}

	w.Header().Set("Content-Type", "application/json")

	var dat []byte

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	storedToken, err := cfg.db.GetRefreshToken(r.Context(), refreshToken)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	if storedToken.RevokedAt.Valid {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	if time.Now().UTC().After(storedToken.ExpiresAt) {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	tokenString, err := auth.MakeJWT(storedToken.UserID, cfg.tokenString, time.Hour)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}

	dat, err = json.Marshal(response{
		Token: tokenString,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email            string `json:"email"`
		Password         string `json:"password"`
		ExpiresInSeconds int    `json:"expires_in_seconds"`
	}
	type errorResponse struct {
		Error string `json:"error"`
	}
	type response struct {
		ID           uuid.UUID `json:"id"`
		Email        string    `json:"email"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}

	w.Header().Set("Content-Type", "application/json")
	decoder := json.NewDecoder(r.Body)
	params := request{}
	err := decoder.Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusBadRequest)})
		w.Write(dat)
		return
	}
	var dat []byte

	user, err := cfg.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}
	found, err := auth.CheckPassword(params.Password, user.HashedPassword)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	if found {
		const maxLifetime = time.Hour

		lifetime := time.Duration(params.ExpiresInSeconds) * time.Second
		if params.ExpiresInSeconds <= 0 || lifetime > maxLifetime {
			lifetime = maxLifetime
		}

		tokenString, err := auth.MakeJWT(user.ID, cfg.tokenString, lifetime)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
			w.Write(dat)
			return
		}

		refreshToken := auth.MakeRefreshToken()
		if refreshToken == "" {
			w.WriteHeader(http.StatusInternalServerError)
			dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
			w.Write(dat)
			return
		}

		now := time.Now().UTC()
		err = cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
			Token:     refreshToken,
			CreatedAt: now,
			UpdatedAt: now,
			UserID:    user.ID,
			ExpiresAt: now.Add(60 * 24 * time.Hour),
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
			w.Write(dat)
			return
		}

		dat, err = json.Marshal(response{
			ID:           user.ID,
			Email:        user.Email,
			CreatedAt:    user.CreatedAt,
			UpdatedAt:    user.UpdatedAt,
			Token:        tokenString,
			RefreshToken: refreshToken,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
			w.Write(dat)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(dat)
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
	}
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
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

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ = json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}
	user, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	})
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

func (cfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	type errorResponse struct {
		Error string `json:"error"`
	}

	w.Header().Set("Content-Type", "application/json")

	tokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}
	userID, err := auth.ValidateJWT(tokenString, cfg.tokenString)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusUnauthorized)})
		w.Write(dat)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := request{}
	err = decoder.Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusBadRequest)})
		w.Write(dat)
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}

	user, err := cfg.db.UpdateUser(r.Context(), database.UpdateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
		ID:             userID,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		dat, _ := json.Marshal(errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
		w.Write(dat)
		return
	}

	dat, err := json.Marshal(User{
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
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	tokenString := os.Getenv("TOKEN_STRING")

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
		tokenString:    tokenString,
	}
	fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux := http.NewServeMux()
	mux.Handle("/app/", apiConfig.middlewareMetricsInc(fileServer))

	mux.HandleFunc("GET /admin/metrics", apiConfig.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiConfig.handlerReset)

	mux.HandleFunc("POST /api/login", apiConfig.handlerLogin)
	mux.HandleFunc("POST /api/refresh", apiConfig.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", apiConfig.handlerRevoke)

	mux.HandleFunc("POST /api/users", apiConfig.handlerCreateUser)
	mux.HandleFunc("PUT /api/users", apiConfig.handlerUpdateUser)

	mux.HandleFunc("GET /api/chirps", apiConfig.handlerGetChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiConfig.handlerGetChirp)
	mux.HandleFunc("POST /api/chirps", apiConfig.handlerCreateChirp)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiConfig.handlerDeleteChirp)

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
