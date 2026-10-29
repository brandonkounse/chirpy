package main

import "net/http"

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", notFoundHandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	_ = http.ListenAndServe(server.Addr, server.Handler)
}
