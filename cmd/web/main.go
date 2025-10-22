package main

import (
	"log"
	"net/http"
)

func main() {
	// Serve static files from the web/static directory
	fs := http.FileServer(http.Dir("web/static"))
	http.Handle("/", fs)

	// Start the server
	port := "3000"
	log.Printf("Web server starting on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
