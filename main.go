package main

import (
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(".")))

	s := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("Running server at http://127.0.0.1:8080/")
	log.Fatal(s.ListenAndServe())
}
