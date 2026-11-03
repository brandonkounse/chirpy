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
	"unicode"

	"github.com/brandonkounse/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
}

func (cfg *apiConfig) addMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) getMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	metrics := fmt.Sprintf(`
<html>
	<body>
		<h1>Welcome, Chirpy Admin</h1>
		<p>Chirpy has been visited %d times!</p>
	</body>
</html>
`, cfg.fileserverHits.Load())
	_, _ = w.Write([]byte(metrics))
}

func (cfg *apiConfig) resetDatabase(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	err := cfg.dbQueries.DeleteUsers(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func sanitizeChirp(chirp string) string {
	profane := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}

	words := strings.Split(chirp, " ")
	for i, word := range words {
		if strings.ContainsFunc(word, unicode.IsPunct) {
			continue
		}
		if _, ok := profane[strings.ToLower(word)]; ok {
			words[i] = "****"
		}
	}
	return strings.Join(words, " ")
}

//type cleanedResponse struct {
//	CleanedBody string `json:"cleaned_body"`
//}

type chirpRequest struct {
	Body   string    `json:"body"`
	UserId uuid.UUID `json:"user_id"`
}

type chirpResponse struct {
	Id        uuid.UUID `json:"id"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	Body      string    `json:"body"`
	UserId    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) chirpHandler(w http.ResponseWriter, r *http.Request) {
	c := chirpRequest{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&c)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Something went wrong"))
		return
	}

	if len([]rune(c.Body)) > 140 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Chirp is too long"))
		return
	}

	c.Body = sanitizeChirp(c.Body)

	chirpParams := database.CreateChirpParams{Body: c.Body, UserID: c.UserId}
	chirp, err := cfg.dbQueries.CreateChirp(r.Context(), chirpParams)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res := chirpResponse{
		Id:        chirp.ID,
		CreatedAt: chirp.CreatedAt.String(),
		UpdatedAt: chirp.UpdatedAt.String(),
		Body:      chirp.Body,
		UserId:    chirp.UserID,
	}

	data, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(data)
}

type createUserRequest struct {
	Email string `json:"email"`
}

type createUserResponse struct {
	Id        uuid.UUID `json:"id"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	Email     string    `json:"email"`
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := cfg.dbQueries.CreateUser(r.Context(), req.Email)
	if err != nil {
		http.Error(w, "error creating user", http.StatusInternalServerError)
		return
	}

	res := createUserResponse{
		Id:        user.ID,
		CreatedAt: user.CreatedAt.String(),
		UpdatedAt: user.UpdatedAt.String(),
		Email:     user.Email,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		http.Error(w, "error encoding response", http.StatusInternalServerError)
	}
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("no .env loaded: %v", err)
	}
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)

	mux := http.NewServeMux()
	api := &apiConfig{
		dbQueries: dbQueries,
		platform:  platform,
	}

	fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))

	mux.Handle("GET /app/", api.addMetrics(fileServer))
	mux.HandleFunc("GET /api/healthz", healthzHandler)
	mux.HandleFunc("GET /admin/metrics", api.getMetrics)
	mux.HandleFunc("POST /admin/reset", api.resetDatabase)
	mux.HandleFunc("POST /api/chirps", api.chirpHandler)
	mux.HandleFunc("POST /api/users", api.createUserHandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	_ = http.ListenAndServe(server.Addr, server.Handler)
}
