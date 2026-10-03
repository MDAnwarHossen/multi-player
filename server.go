package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"lunarfall/game"

	"github.com/gorilla/websocket"
)

type gameServer struct {
	engine    *game.Engine
	clientsMu sync.Mutex
	clients   map[*client]struct{}
	upgrader  websocket.Upgrader
}

type client struct {
	connection *websocket.Conn
	playerID   string
	outbound   chan response
	done       chan struct{}
	closeOnce  sync.Once
}

type message struct {
	Type      string   `json:"type"`
	Name      string   `json:"name,omitempty"`
	Race      string   `json:"race,omitempty"`
	MapID     string   `json:"mapId,omitempty"`
	Ready     bool     `json:"ready,omitempty"`
	X         float64  `json:"x,omitempty"`
	Y         float64  `json:"y,omitempty"`
	EntityIDs []string `json:"entityIds,omitempty"`
	EntityID  string   `json:"entityId,omitempty"`
	NodeID    string   `json:"nodeId,omitempty"`
	TargetID  string   `json:"targetId,omitempty"`
	Kind      string   `json:"kind,omitempty"`
}

type response struct {
	Type     string         `json:"type"`
	PlayerID string         `json:"playerId,omitempty"`
	Error    string         `json:"error,omitempty"`
	State    *game.Snapshot `json:"state,omitempty"`
}

func newGameServer(engine *game.Engine) *gameServer {
	server := &gameServer{engine: engine, clients: make(map[*client]struct{})}
	server.upgrader = websocket.Upgrader{CheckOrigin: server.allowOrigin}
	return server
}

func (server *gameServer) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	connection, err := server.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	connected := &client{connection: connection, outbound: make(chan response, 8), done: make(chan struct{})}
	go connected.writePump()
	server.clientsMu.Lock()
	server.clients[connected] = struct{}{}
	server.clientsMu.Unlock()
	defer func() {
		server.clientsMu.Lock()
		delete(server.clients, connected)
		server.clientsMu.Unlock()
		if connected.playerID != "" {
			server.engine.Leave(connected.playerID)
			server.broadcast()
		}
		connected.close()
	}()

	connection.SetReadLimit(4096)
	if err := connected.send(response{Type: "snapshot", State: snapshotPointer(server.engine.Snapshot())}); err != nil {
		return
	}
	for {
		var incoming message
		if err := connection.ReadJSON(&incoming); err != nil {
			return
		}
		if connected.playerID == "" {
			if incoming.Type != "join" {
				connected.send(response{Type: "error", Error: "join with a player name first"})
				continue
			}
			if utf8.RuneCountInString(strings.TrimSpace(incoming.Name)) > 18 {
				connected.send(response{Type: "error", Error: "player names must be 18 characters or fewer"})
				continue
			}
			id, err := newPlayerID()
			if err == nil {
				err = server.engine.Join(id, incoming.Name)
			}
			if err != nil {
				connected.send(response{Type: "error", Error: err.Error()})
				continue
			}
			connected.playerID = id
			connected.send(response{Type: "joined", PlayerID: id})
			server.broadcast()
			continue
		}

		if err := server.apply(connected.playerID, incoming); err != nil {
			connected.send(response{Type: "error", Error: err.Error()})
			continue
		}
		if incoming.Type == "quit" {
			connected.send(response{Type: "left"})
			server.broadcast()
			return
		}
		server.broadcast()
	}
}

func (server *gameServer) apply(playerID string, incoming message) error {
	switch incoming.Type {
	case "computer":
		return server.engine.AddComputer(playerID)
	case "map":
		return server.engine.SetMap(playerID, incoming.MapID)
	case "race":
		return server.engine.SetRace(playerID, incoming.Race)
	case "ready":
		return server.engine.SetReady(playerID, incoming.Ready)
	case "start":
		return server.engine.Start(playerID)
	case "move":
		return server.engine.Move(playerID, incoming.EntityIDs, incoming.X, incoming.Y)
	case "gather":
		return server.engine.Gather(playerID, incoming.EntityID, incoming.NodeID)
	case "build":
		return server.engine.Build(playerID, incoming.EntityID, incoming.Kind, incoming.X, incoming.Y)
	case "train":
		return server.engine.Train(playerID, incoming.EntityID, incoming.Kind)
	case "research":
		return server.engine.Research(playerID, incoming.EntityID, incoming.Kind)
	case "attack":
		return server.engine.Attack(playerID, incoming.EntityIDs, incoming.TargetID)
	case "pause":
		return server.engine.Pause(playerID)
	case "resume":
		return server.engine.Resume(playerID)
	case "quit":
		return server.engine.Quit(playerID)
	default:
		return errors.New("unknown game action")
	}
}

func (server *gameServer) broadcast() {
	state := snapshotPointer(server.engine.Snapshot())
	server.clientsMu.Lock()
	clients := make([]*client, 0, len(server.clients))
	for connected := range server.clients {
		clients = append(clients, connected)
	}
	server.clientsMu.Unlock()
	for _, connected := range clients {
		if err := connected.send(response{Type: "snapshot", State: state}); err != nil {
			connected.close()
		}
	}
}

func (server *gameServer) runTicks(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		server.engine.Tick(interval)
		server.broadcast()
	}
}

func (connected *client) send(outgoing response) error {
	select {
	case <-connected.done:
		return errors.New("connection is closed")
	case connected.outbound <- outgoing:
		return nil
	default:
		return errors.New("connection cannot keep up with game updates")
	}
}

func (connected *client) writePump() {
	for {
		select {
		case <-connected.done:
			return
		case outgoing := <-connected.outbound:
			if err := connected.connection.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
				connected.close()
				return
			}
			if err := connected.connection.WriteJSON(outgoing); err != nil {
				connected.close()
				return
			}
		}
	}
}

func (connected *client) close() {
	connected.closeOnce.Do(func() {
		close(connected.done)
		connected.connection.Close()
	})
}

func (server *gameServer) allowOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if configured := strings.Split(os.Getenv("ALLOWED_ORIGINS"), ","); len(configured) > 1 || configured[0] != "" {
		for _, allowed := range configured {
			if strings.TrimRight(strings.TrimSpace(allowed), "/") == strings.TrimRight(origin, "/") {
				return true
			}
		}
		return false
	}
	parsedOrigin, err := url.Parse(origin)
	if err != nil || parsedOrigin.Hostname() == "" {
		return false
	}
	requestHost, _, splitErr := net.SplitHostPort(request.Host)
	if splitErr != nil {
		requestHost = request.Host
	}
	originHost := strings.ToLower(parsedOrigin.Hostname())
	return originHost == strings.ToLower(requestHost) || originHost == "localhost" || originHost == "127.0.0.1" || originHost == "::1"
}

func newPlayerID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("create player id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func snapshotPointer(snapshot game.Snapshot) *game.Snapshot {
	return &snapshot
}

func frontendHandler() http.Handler {
	frontend, err := fs.Sub(os.DirFS("frontend/dist"), ".")
	if err != nil {
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "frontend is not built; run npm run build in frontend", http.StatusServiceUnavailable)
		})
	}
	if _, err := fs.Stat(frontend, "index.html"); err != nil {
		log.Printf("frontend unavailable: %v", err)
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "frontend is not built; run npm run build in frontend", http.StatusServiceUnavailable)
		})
	}
	return http.FileServer(http.FS(frontend))
}

var _ http.Handler = http.HandlerFunc(nil)
