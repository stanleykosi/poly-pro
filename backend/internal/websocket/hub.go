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
	"log/slog"

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
			h.logger.Info("new client registered", "remote_addr", client.Conn.RemoteAddr())
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
			if _, ok := h.subscriptions[sub.marketID]; !ok {
				h.subscriptions[sub.marketID] = make(map[*Client]bool)
				// First client for this market, so we subscribe to the Redis channel.
				go h.listenToMarket(sub.marketID)
			}
			h.subscriptions[sub.marketID][sub.client] = true
			h.logger.Info("client subscribed to market", "market_id", sub.marketID, "client", sub.client.Conn.RemoteAddr())
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

	h.logger.Info("subscribing to redis channel", "channel", channel)

	ch := pubsub.Channel()

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("stopping redis listener for channel", "channel", channel)
			return
		case msg := <-ch:
			h.broadcastToMarket(marketID, []byte(msg.Payload))
		}
	}
}

// broadcastToMarket sends a message to all clients subscribed to a specific market.
func (h *Hub) broadcastToMarket(marketID string, message []byte) {
	if market, ok := h.subscriptions[marketID]; ok {
		h.logger.Debug("broadcasting to market", "market_id", marketID, "num_clients", len(market))
		for client := range market {
			select {
			case client.Send <- message:
			default:
				// If the client's send buffer is full, assume it's slow or disconnected.
				// Unregister the client to prevent blocking.
				close(client.Send)
				delete(h.clients, client)
				// Also remove from the specific subscription
				delete(market, client)
			}
		}
	}
}

