package websocket

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins. In production, you might want to restrict this.
	},
}

// Client represents a WebSocket client
type Client struct {
	ID       string
	Nickname string
	Room     string
	Conn     *websocket.Conn
	Send     chan []byte
}

// Message represents a message structure
type Message struct {
	Type     string      `json:"type"`
	Content  interface{} `json:"content"`
	Room     string      `json:"room"`
	Nickname string      `json:"nickname"`
}

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	rooms      map[string]map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mutex      *sync.Mutex
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		rooms:      make(map[string]map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		mutex:      &sync.Mutex{},
	}
}

// Run starts the Hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			if _, ok := h.rooms[client.Room]; !ok {
				h.rooms[client.Room] = make(map[*Client]bool)
			}
			h.rooms[client.Room][client] = true

			// Send current users list to all clients in the room
			users := []string{}
			for c := range h.rooms[client.Room] {
				users = append(users, c.Nickname)
			}
			usersUpdate := Message{
				Type:    "users_update",
				Content: users,
				Room:    client.Room,
			}
			if usersBytes, err := json.Marshal(usersUpdate); err == nil {
				for c := range h.rooms[client.Room] {
					select {
					case c.Send <- usersBytes:
					default:
						close(c.Send)
						delete(h.rooms[client.Room], c)
					}
				}
			}

			// Send join message
			joinMsg := Message{
				Type:     "system",
				Content:  client.Nickname + " joined the room",
				Room:     client.Room,
				Nickname: "System",
			}
			msgBytes, _ := json.Marshal(joinMsg)
			for c := range h.rooms[client.Room] {
				select {
				case c.Send <- msgBytes:
				default:
					close(c.Send)
					delete(h.rooms[client.Room], c)
				}
			}
			h.mutex.Unlock()

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.rooms[client.Room]; ok {
				if _, ok := h.rooms[client.Room][client]; ok {
					delete(h.rooms[client.Room], client)
					close(client.Send)

					// Send leave message
					leaveMsg := Message{
						Type:     "system",
						Content:  client.Nickname + " left the room",
						Room:     client.Room,
						Nickname: "System",
					}
					msgBytes, _ := json.Marshal(leaveMsg)
					for c := range h.rooms[client.Room] {
						select {
						case c.Send <- msgBytes:
						default:
							close(c.Send)
							delete(h.rooms[client.Room], c)
						}
					}

					// Send updated users list
					users := []string{}
					for c := range h.rooms[client.Room] {
						users = append(users, c.Nickname)
					}
					usersUpdate := Message{
						Type:    "users_update",
						Content: users,
						Room:    client.Room,
					}
					if usersBytes, err := json.Marshal(usersUpdate); err == nil {
						for c := range h.rooms[client.Room] {
							select {
							case c.Send <- usersBytes:
							default:
								close(c.Send)
								delete(h.rooms[client.Room], c)
							}
						}
					}

					if len(h.rooms[client.Room]) == 0 {
						delete(h.rooms, client.Room)
					}
				}
			}
			h.mutex.Unlock()

		case message := <-h.broadcast:
			h.mutex.Lock()
			var msg Message
			if err := json.Unmarshal(message, &msg); err == nil {
				if room, ok := h.rooms[msg.Room]; ok {
					for client := range room {
						select {
						case client.Send <- message:
						default:
							close(client.Send)
							delete(h.rooms[msg.Room], client)
						}
					}
				}
			}
			h.mutex.Unlock()
		}
	}
}

func (c *Client) readPump(hub *Hub) {
	defer func() {
		hub.unregister <- c
		c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Errorf("error: %v", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err == nil {
			// Always ensure nickname is set from the client
			msg.Nickname = c.Nickname
			msg.Room = c.Room // Ensure room is set correctly

			// Prepare the message for broadcasting
			msgBytes, err := json.Marshal(msg)
			if err != nil {
				log.Errorf("Failed to marshal message: %v", err)
				continue
			}

			// For cursor updates, drawing, and code updates, broadcast directly to room
			if msg.Type == "cursor_update" || msg.Type == "cursor_move" ||
				msg.Type == "draw" || msg.Type == "code_update" ||
				msg.Type == "clear" {
				if room, ok := hub.rooms[c.Room]; ok {
					for client := range room {
						select {
						case client.Send <- msgBytes:
						default:
							close(client.Send)
							delete(hub.rooms[c.Room], client)
						}
					}
				}
			} else {
				// For other messages, use the general broadcast channel
				hub.broadcast <- msgBytes
			}
		}
	}
}

func (c *Client) writePump() {
	defer func() {
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		}
	}
}

// ServeWs handles WebSocket requests from the peer
func ServeWs(hub *Hub, c *gin.Context) {
	log.Info("Received WebSocket connection request")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Errorf("Failed to upgrade connection to WebSocket: %v", err)
		return
	}
	log.Info("WebSocket connection established")

	client := &Client{
		ID:       c.Query("id"),
		Nickname: c.Query("nickname"),
		Room:     c.Query("room"),
		Conn:     conn,
		Send:     make(chan []byte, 256),
	}

	hub.register <- client

	go client.writePump()
	go client.readPump(hub)
}

// BroadcastMessage sends a message to all connected clients
func (h *Hub) BroadcastMessage(messageType string, content interface{}) {
	message := Message{
		Type:     messageType,
		Content:  content,
		Nickname: "System",
	}
	if msgBytes, err := json.Marshal(message); err == nil {
		h.broadcast <- msgBytes
	}
}

// InitWebSocketModule initializes the WebSocket module
func InitWebSocketModule(router *gin.RouterGroup) *Hub {
	hub := NewHub()
	go hub.Run()
	SetupWebSocketRoutes(router, hub)
	return hub
}

// SetupWebSocketRoutes sets up the WebSocket routes
func SetupWebSocketRoutes(router *gin.RouterGroup, hub *Hub) {
	router.GET("/ws", WebSocketHandler(hub))
}

// WebSocketHandler returns a gin.HandlerFunc for handling WebSocket connections
// @Summary Connect to WebSocket
// @Description Establishes a WebSocket connection, check example at: /static/chat.html
// @Security ApiKeyAuth
// @Security BearerAuth
// @Tags Core/Websocket
// @Accept  json
// @Produce  json
// @Param id query string false "Client ID"
// @Param nickname query string false "User Nickname"
// @Param room query string false "Chat Room"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} ErrorResponse
// @Router /ws [get]
func WebSocketHandler(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		ServeWs(hub, c)
	}
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}
