/**
 * @description
 * This file defines the WebSocket `Hub`, which acts as a central manager for all active
 * client connections. It orchestrates client registration, unregistration, and the
 * broadcasting of real-time market data.
 *
 * Key features:
 * - Connection Management: Maintains a registry of all connected clients.
 * - Channel-based Communication: Uses channels for concurrent and safe handling of
 *   client registrations, unregistrations, and messages.
 * - Market Subscriptions: Manages which clients are subscribed to which market data streams.
 * - Redis Pub/Sub Integration: Subscribes to Redis channels to receive market data updates
 *   from backend services (like the `MarketStreamService`).
 * - Fan-Out Broadcasting: Efficiently broadcasts incoming data from Redis to all relevant
 *   subscribed clients.
 *
 * @dependencies
 * - github.com/redis/go-redis/v9: The Redis client library.
 * - log/slog: For structured logging.
 *
 * @notes
 * - The `Run` method is the heart of the hub, running in a continuous loop to process
 *   events from all channels. It should be started as a goroutine when the application launches.
 * - This architecture decouples the data source (e.g., Polymarket's WSS feed via a service)
 *   from the data consumers (the connected WebSocket clients), allowing for a scalable
 *   real-time infrastructure.
 */

package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// subscription represents a client's subscription to a specific market.
type subscription struct {
	client   *Client
	marketID string
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool
	// Register requests from the clients.
	Register chan *Client
	// Unregister requests from clients.
	Unregister chan *Client
	// Subscription requests from clients.
	Subscribe chan subscription
	// Unsubscription requests from clients.
	Unsubscribe chan subscription
	// Map of marketID to a set of subscribed clients.
	subscriptions map[string]map[*Client]bool
	// Redis client for Pub/Sub.
	redisClient *redis.Client
	logger      *slog.Logger
	ctx         context.Context
}

// NewHub creates a new Hub instance.
func NewHub(ctx context.Context, logger *slog.Logger, redisClient *redis.Client) *Hub {
	return &Hub{
		clients:       make(map[*Client]bool),
		Register:      make(chan *Client),
		Unregister:    make(chan *Client),
		Subscribe:     make(chan subscription),
		Unsubscribe:   make(chan subscription),
		subscriptions: make(map[string]map[*Client]bool),
		redisClient:   redisClient,
		logger:        logger,
		ctx:           ctx,
	}
}

// Run starts the hub's event loop. It should be run in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("hub shutting down")
			// Close all client connections
			for client := range h.clients {
				close(client.Send)
				delete(h.clients, client)
			}
			return
		case client := <-h.Register:
			h.clients[client] = true
			h.logger.Info("✅ hub: new client registered", "remote_addr", client.Conn.RemoteAddr(), "total_clients", len(h.clients))
		case client := <-h.Unregister:
			if _, ok := h.clients[client]; ok {
				// Remove client from all its subscriptions
				for marketID := range client.Subscriptions {
					if market, ok := h.subscriptions[marketID]; ok {
						delete(market, client)
						if len(market) == 0 {
							delete(h.subscriptions, marketID)
							// Optionally, unsubscribe from the Redis channel if no clients are listening.
							// For simplicity, we can leave the Redis subscription open.
						}
					}
				}
				delete(h.clients, client)
				close(client.Send)
				h.logger.Info("client unregistered", "remote_addr", client.Conn.RemoteAddr())
			}
		case sub := <-h.Subscribe:
			h.logger.Info("📨 hub: received subscription request", 
				"market_id", sub.marketID, 
				"market_id_length", len(sub.marketID),
				"market_id_hex", fmt.Sprintf("%x", []byte(sub.marketID)),
				"market_id_bytes", []byte(sub.marketID),
				"client_addr", sub.client.Conn.RemoteAddr())
			if _, ok := h.subscriptions[sub.marketID]; !ok {
				h.subscriptions[sub.marketID] = make(map[*Client]bool)
				// First client for this market, so we subscribe to the Redis channel.
				h.logger.Info("🆕 hub: first subscription to market, starting Redis listener", 
					"market_id", sub.marketID,
					"market_id_hex", fmt.Sprintf("%x", []byte(sub.marketID)),
					"redis_channel", "market:"+sub.marketID)
				go h.listenToMarket(sub.marketID)
			}
			h.subscriptions[sub.marketID][sub.client] = true
			// Verify the subscription was stored correctly
			if storedMarket, ok := h.subscriptions[sub.marketID]; ok {
				h.logger.Info("✅ hub: client subscribed to market", 
					"market_id", sub.marketID,
					"market_id_hex", fmt.Sprintf("%x", []byte(sub.marketID)),
					"client", sub.client.Conn.RemoteAddr(), 
					"total_clients_for_market", len(storedMarket), 
					"all_subscriptions", len(h.subscriptions),
					"subscription_verified", true)
			} else {
				h.logger.Error("❌ hub: subscription NOT found after storing!", 
					"market_id", sub.marketID,
					"market_id_hex", fmt.Sprintf("%x", []byte(sub.marketID)))
			}
		case sub := <-h.Unsubscribe:
			if market, ok := h.subscriptions[sub.marketID]; ok {
				delete(market, sub.client)
				if len(market) == 0 {
					delete(h.subscriptions, sub.marketID)
					// Again, optionally unsubscribe from Redis here.
				}
				h.logger.Info("client unsubscribed from market", "market_id", sub.marketID, "client", sub.client.Conn.RemoteAddr())
			}
		}
	}
}

// listenToMarket subscribes to a specific market's Redis channel and broadcasts messages.
func (h *Hub) listenToMarket(marketID string) {
	channel := "market:" + marketID
	pubsub := h.redisClient.Subscribe(h.ctx, channel)
	defer pubsub.Close()

	h.logger.Info("subscribing to redis channel", 
		"channel", channel, 
		"market_id", marketID,
		"market_id_length", len(marketID),
		"market_id_bytes", []byte(marketID))

	ch := pubsub.Channel()
	messageCount := 0

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("stopping redis listener for channel", "channel", channel)
			return
		case msg := <-ch:
			messageCount++
			if messageCount == 1 {
				h.logger.Info("✅ hub: received first message from Redis", 
					"channel", channel, 
					"market_id", marketID,
					"market_id_length", len(marketID),
					"payload_size", len(msg.Payload))
			}
			if messageCount%100 == 0 {
				h.logger.Info("hub: messages from Redis", "channel", channel, "count", messageCount)
			}
			// Log first few messages to verify they're being received
			if messageCount <= 3 {
				var msgData map[string]interface{}
				if err := json.Unmarshal([]byte(msg.Payload), &msgData); err == nil {
					h.logger.Info("hub: message content", 
						"market_id", marketID, 
						"message_market_field", msgData["market"],
						"event_type", msgData["event_type"], 
						"has_bids", msgData["bids"] != nil, 
						"has_asks", msgData["asks"] != nil)
				}
			}
			h.broadcastToMarket(marketID, []byte(msg.Payload))
		}
	}
}

// broadcastToMarket sends a message to all clients subscribed to a specific market.
var broadcastCallCount int64

func (h *Hub) broadcastToMarket(marketID string, message []byte) {
	// Log the exact marketID being used for lookup
	callCount := atomic.AddInt64(&broadcastCallCount, 1)
	if callCount <= 5 {
		h.logger.Info("🔍 hub: broadcastToMarket called", 
			"market_id", marketID,
			"market_id_length", len(marketID),
			"market_id_hex", fmt.Sprintf("%x", []byte(marketID)),
			"subscriptions_map_size", len(h.subscriptions),
			"call_count", callCount)
	}
	
	if market, ok := h.subscriptions[marketID]; ok {
		if len(market) > 0 {
			// Log first few broadcasts to verify messages are being sent
			if callCount <= 5 {
				h.logger.Info("✅ hub: found subscription, broadcasting", 
					"market_id", marketID, 
					"num_clients", len(market), 
					"message_size", len(message),
					"call_count", callCount)
			}
		}
		for client := range market {
			select {
			case client.Send <- message:
				// Message sent successfully
			default:
				// If the client's send buffer is full, assume it's slow or disconnected.
				// Unregister the client to prevent blocking.
				h.logger.Warn("client send buffer full, unregistering", "market_id", marketID, "client", client.Conn.RemoteAddr())
				close(client.Send)
				delete(h.clients, client)
				// Also remove from the specific subscription
				delete(market, client)
			}
		}
	} else {
		// Log all subscribed market IDs to help debug mismatches
		subscribedMarkets := make([]string, 0, len(h.subscriptions))
		subscribedMarketsHex := make([]string, 0, len(h.subscriptions))
		for mID := range h.subscriptions {
			subscribedMarkets = append(subscribedMarkets, mID)
			subscribedMarketsHex = append(subscribedMarketsHex, fmt.Sprintf("%x", []byte(mID)))
		}
		h.logger.Warn("❌ hub: no clients subscribed to market", 
			"market_id", marketID, 
			"market_id_length", len(marketID),
			"market_id_hex", fmt.Sprintf("%x", []byte(marketID)),
			"available_markets", len(h.subscriptions),
			"subscribed_markets", subscribedMarkets,
			"subscribed_markets_hex", subscribedMarketsHex)
	}
}

