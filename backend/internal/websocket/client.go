/**
 * @description
 * This file defines the `Client` struct, which represents a single user's WebSocket connection
 * to the server. It manages the lifecycle of the connection, including reading incoming
 * messages and writing outgoing messages.
 *
 * Key features:
 * - Connection Management: Wraps a `gorilla/websocket` connection.
 * - Concurrency: Uses channels and goroutines for non-blocking read and write operations.
 * - Subscription Handling: Maintains a set of market IDs that the client is subscribed to.
 * - Graceful Shutdown: The read and write pumps are designed to clean up and unregister
 *   the client when the connection is closed.
 *
 * @dependencies
 * - github.com/gorilla/websocket: The WebSocket library used for connection handling.
 * - log/slog: For structured logging.
 *
 * @notes
 * - The `readPump` and `writePump` methods run in separate goroutines for each client,
 *   allowing the server to handle many concurrent connections efficiently.
 * - The client communicates with the `Hub` via channels for registration, unregistration,
 *   and message broadcasting.
 */

package websocket

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	Hub          *Hub
	Conn         *websocket.Conn
	Send         chan []byte
	Subscriptions map[string]bool
	Logger       *slog.Logger
}

// subscriptionMessage defines the structure for incoming subscription requests from the client.
type subscriptionMessage struct {
	Type      string   `json:"type"` // e.g., "subscribe", "unsubscribe"
	MarketIDs []string `json:"market_ids"`
}

// ReadPump pumps messages from the websocket connection to the hub.
// The application runs ReadPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	messageCount := 0
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Logger.Error("unexpected websocket close error", "error", err)
			}
			break
		}
		messageCount++
		if messageCount == 1 {
			c.Logger.Info("✅ client: received first message", "remote_addr", c.Conn.RemoteAddr(), "message_size", len(message), "message_preview", string(message[:min(len(message), 200)]))
		}
		c.handleMessage(message)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleMessage processes incoming messages from the client, such as subscription requests.
func (c *Client) handleMessage(message []byte) {
	c.Logger.Info("🔍 client: processing message", "remote_addr", c.Conn.RemoteAddr(), "message_size", len(message), "raw_message", string(message))
	
	var msg subscriptionMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		c.Logger.Warn("❌ client: failed to unmarshal message", "error", err, "message", string(message), "remote_addr", c.Conn.RemoteAddr())
		return
	}

	c.Logger.Info("✅ client: parsed message", "type", msg.Type, "markets", msg.MarketIDs, "markets_count", len(msg.MarketIDs), "client_addr", c.Conn.RemoteAddr())

	switch msg.Type {
	case "subscribe":
		for _, marketID := range msg.MarketIDs {
			if !c.Subscriptions[marketID] {
				c.Subscriptions[marketID] = true
				c.Logger.Info("📥 client: sending subscription to hub", "market_id", marketID, "client_addr", c.Conn.RemoteAddr())
				c.Hub.Subscribe <- subscription{client: c, marketID: marketID}
			} else {
				c.Logger.Debug("client already subscribed to market", "market_id", marketID)
			}
		}
	case "unsubscribe":
		for _, marketID := range msg.MarketIDs {
			if c.Subscriptions[marketID] {
				delete(c.Subscriptions, marketID)
				c.Hub.Unsubscribe <- subscription{client: c, marketID: marketID}
			}
		}
	default:
		c.Logger.Warn("received unknown message type from client", "type", msg.Type)
	}
}

// WritePump pumps messages from the hub to the websocket connection.
// A goroutine running WritePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	messageCount := 0
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			messageCount++
			if messageCount == 1 {
				// Log first message content to verify it's being sent
				var msgData map[string]interface{}
				if err := json.Unmarshal(message, &msgData); err == nil {
					c.Logger.Info("✅ client: sending first message to WebSocket", 
						"remote_addr", c.Conn.RemoteAddr(),
						"event_type", msgData["event_type"],
						"market", msgData["market"],
						"message_size", len(message))
				} else {
					c.Logger.Info("✅ client: sending first message to WebSocket", "remote_addr", c.Conn.RemoteAddr(), "message_size", len(message))
				}
			}
			if messageCount%100 == 0 {
				c.Logger.Info("client: messages sent", "remote_addr", c.Conn.RemoteAddr(), "count", messageCount)
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.Logger.Error("failed to get next writer", "error", err)
				return
			}
			w.Write(message)

			// Add queued messages to the current websocket message.
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				c.Logger.Error("failed to close writer", "error", err)
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

