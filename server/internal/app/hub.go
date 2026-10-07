package app

import (
	"encoding/json"
	"sync"
)

type socketClient struct {
	user User
	send chan []byte
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*socketClient]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[*socketClient]struct{})}
}

func (h *Hub) Subscribe(roomCode string, client *socketClient) func() {
	h.mu.Lock()
	if h.clients[roomCode] == nil {
		h.clients[roomCode] = make(map[*socketClient]struct{})
	}
	h.clients[roomCode][client] = struct{}{}
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.clients[roomCode], client)
		if len(h.clients[roomCode]) == 0 {
			delete(h.clients, roomCode)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Broadcast(roomCode string, envelope Envelope) {
	message, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	h.mu.RLock()
	clients := make([]*socketClient, 0, len(h.clients[roomCode]))
	for client := range h.clients[roomCode] {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	for _, client := range clients {
		select {
		case client.send <- message:
		default:
			// A slow client will recover from the next authoritative snapshot.
		}
	}
}
