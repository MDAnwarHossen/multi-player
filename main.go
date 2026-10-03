package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"lunarfall/game"
)

func main() {
	engine := game.NewEngine()
	server := newGameServer(engine)
	go server.runTicks(50 * time.Millisecond)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.handleWebSocket)
	mux.HandleFunc("/healthz", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", frontendHandler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Lunarfall listening on 0.0.0.0:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
