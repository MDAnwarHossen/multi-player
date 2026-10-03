package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lunarfall/game"

	"github.com/gorilla/websocket"
)

func TestWebSocketLobbyJoinAndHostAuthorization(t *testing.T) {
	server := newGameServer(game.NewEngine())
	httpServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer httpServer.Close()
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	host := connectPlayer(t, websocketURL, "Fern")
	defer host.Close()
	guest := connectPlayer(t, websocketURL, "Moss")
	defer guest.Close()
	if err := guest.WriteJSON(message{Type: "race", Race: "alien"}); err != nil {
		t.Fatal(err)
	}
	for {
		packet := readPacketType(t, guest, "snapshot")
		foundAlien := false
		for _, player := range packet.State.Players {
			if player.Name == "Moss" && player.Race == "alien" {
				foundAlien = true
			}
		}
		if foundAlien {
			break
		}
	}

	if err := guest.WriteJSON(message{Type: "start"}); err != nil {
		t.Fatal(err)
	}
	packet := readPacketType(t, guest, "error")
	if packet.Type != "error" || packet.Error != "only the host can start the match" {
		t.Fatalf("expected host-only start rejection, got %+v", packet)
	}

	duplicate := openConnection(t, websocketURL)
	defer duplicate.Close()
	if err := duplicate.WriteJSON(message{Type: "join", Name: "fern"}); err != nil {
		t.Fatal(err)
	}
	packet = readPacketType(t, duplicate, "error")
	if packet.Type != "error" || packet.Error != "that player name is already in use" {
		t.Fatalf("expected case-insensitive duplicate rejection, got %+v", packet)
	}
}

func TestWebSocketHostAddsComputerOpponent(t *testing.T) {
	server := newGameServer(game.NewEngine())
	httpServer := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer httpServer.Close()
	websocketURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	host := connectPlayer(t, websocketURL, "Vega")
	defer host.Close()
	if err := host.WriteJSON(message{Type: "computer"}); err != nil {
		t.Fatal(err)
	}
	for {
		packet := readPacketType(t, host, "snapshot")
		for _, player := range packet.State.Players {
			if player.Computer {
				if !player.Ready || player.Race != "alien" {
					t.Fatalf("computer should be ready as the opposing faction: %+v", player)
				}
				return
			}
		}
	}
}

func connectPlayer(t *testing.T, address, name string) *websocket.Conn {
	t.Helper()
	connection := openConnection(t, address)
	if err := connection.WriteJSON(message{Type: "join", Name: name}); err != nil {
		t.Fatal(err)
	}
	for {
		packet := readPacket(t, connection)
		if packet.Type == "joined" {
			return connection
		}
		if packet.Type == "error" {
			t.Fatalf("joining %q failed: %s", name, packet.Error)
		}
	}
}

func openConnection(t *testing.T, address string) *websocket.Conn {
	t.Helper()
	connection, _, err := websocket.DefaultDialer.Dial(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	readPacket(t, connection)
	return connection
}

func readPacket(t *testing.T, connection *websocket.Conn) response {
	t.Helper()
	var packet response
	if err := connection.ReadJSON(&packet); err != nil {
		t.Fatal(err)
	}
	return packet
}

func readPacketType(t *testing.T, connection *websocket.Conn, expected string) response {
	t.Helper()
	for {
		packet := readPacket(t, connection)
		if packet.Type == expected {
			return packet
		}
	}
}
