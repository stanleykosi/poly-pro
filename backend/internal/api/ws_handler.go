/**
 * @description
 * This file contains the Gin HTTP handler for upgrading a standard HTTP connection
 * to a WebSocket connection. It serves as the entry point for all real-time clients.
 *
 * Key features:
 * - WebSocket Upgrade: Uses the `gorilla/websocket` library's `Upgrader` to handle
 *   the WebSocket handshake protocol.
 * - Client Instantiation: Once a connection is successfully upgraded, it creates a new
 *   `Client` instance to manage the connection.
 * - Hub Registration: The new client is registered with the central `Hub`, allowing it
 *   to receive broadcasted messages.
 * - Goroutine Management: Starts the `ReadPump` and `WritePump` for the new client in
 *   separate goroutines, enabling concurrent, non-blocking communication.
 *
 * @dependencies
 * - github.com/gin-gonic/gin: The web framework.
 * - github.com/gorilla/websocket: The WebSocket library.
 * - log/slog: For structured logging.
 * - net/http: For HTTP status codes and request/response writing.
 * - github.com/poly-pro/backend/internal/websocket: For the Client and Hub types.
 */
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	gorillaWS "github.com/gorilla/websocket"
	"github.com/poly-pro/backend/internal/websocket"
)

// upgrader configures the parameters for upgrading an HTTP connection to a WebSocket connection.
var upgrader = gorillaWS.Upgrader{
	// CheckOrigin allows us to control which origins are allowed to connect.
	// For development, we allow all origins. In production, this should be
	// configured to only allow requests from the frontend's domain.
	CheckOrigin: func(r *http.Request) bool {
		// TODO: In production, validate the origin against a list of allowed domains.
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// serveWs handles websocket requests from the peer.
func (server *Server) serveWs(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		server.logger.Error("failed to upgrade connection to websocket", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to upgrade connection"})
		return
	}

	// Create a new client for this connection.
	client := &websocket.Client{
		Hub:          server.hub,
		Conn:         conn,
		Send:         make(chan []byte, 256),
		Subscriptions: make(map[string]bool),
		Logger:       server.logger,
	}

	// Register the new client with the hub.
	server.logger.Info("🔌 ws_handler: client created, registering with hub", "remote_addr", conn.RemoteAddr())
	server.hub.Register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.WritePump()
	go client.ReadPump()

	server.logger.Info("✅ ws_handler: websocket client connected and pumps started", "remote_addr", conn.RemoteAddr())
}

