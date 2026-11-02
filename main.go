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
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
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

func (cfg *apiConfig) resetMetrics(_ http.ResponseWriter, _ *http.Request) {
	cfg.fileserverHits.Store(0)
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

type Chirp struct {
	Body string `json:"body"`
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

type cleanedResponse struct {
	CleanedBody string `json:"cleaned_body"`
}

func validateChirp(w http.ResponseWriter, r *http.Request) {
	c := Chirp{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&c)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Something went wrong"))
	}

	if len([]rune(c.Body)) > 140 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Chirp is too long"))
		return
	}

	data, err := json.Marshal(cleanedResponse{CleanedBody: sanitizeChirp(c.Body)})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Something went wrong"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func main() {
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)

	mux := http.NewServeMux()
	api := &apiConfig{
		dbQueries: dbQueries,
	}

	fileServer := http.StripPrefix("/app", http.FileServer(http.Dir(".")))

	mux.Handle("GET /app/", api.addMetrics(fileServer))
	mux.HandleFunc("GET /api/healthz", healthzHandler)
	mux.HandleFunc("GET /admin/metrics", api.getMetrics)
	mux.HandleFunc("POST /admin/reset", api.resetMetrics)
	mux.HandleFunc("POST /api/validate_chirp", validateChirp)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	_ = http.ListenAndServe(server.Addr, server.Handler)
}
