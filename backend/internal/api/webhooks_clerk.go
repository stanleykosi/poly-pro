/**
 * @description
 * This file contains the HTTP handler for processing webhooks from Clerk.
 * Specifically, it handles the 'user.created' event to synchronize new users
 * with the application's own database.
 *
 * Key features:
 * - Secure Webhook Verification: Implements Svix signature verification to verify the
 *   authenticity of incoming webhooks using HMAC-SHA256 with the secret key.
 * - Decoupled Logic: The handler is responsible only for the HTTP-level interaction
 *   (request/response), while the actual business logic of creating a user
 *   is delegated to the UserService.
 * - Robust Error Handling: Provides clear JSON error responses for various failure
 *   scenarios, such as invalid request body, failed verification, or database errors.
 */

package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/poly-pro/backend/internal/services"
)

// clerkUserCreatedEvent represents the structure of the relevant parts of a
// Clerk 'user.created' webhook payload. We only unmarshal the fields we need.
type clerkUserCreatedEvent struct {
	Data struct {
		ID            string `json:"id"`
		EmailAddresses []struct {
			EmailAddress string `json:"email_address"`
			ID           string `json:"id"`
		} `json:"email_addresses"`
	} `json:"data"`
	Type string `json:"type"`
}

/**
 * @description
 * handleCreateUserWebhook is a Gin handler that processes incoming webhooks from Clerk.
 * It verifies the webhook's signature and, if it's a 'user.created' event,
 * it creates a new user in the database.
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - The Clerk Webhook Secret must be configured in the environment variables.
 * - This endpoint should be registered in the Clerk Dashboard for the 'user.created' event.
 * - It relies on the Svix headers (svix-id, svix-timestamp, svix-signature)
 *   being present in the request for verification.
 */
func (server *Server) handleCreateUserWebhook(c *gin.Context) {
	// 1. Read the request body.
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		server.logger.Error("failed to read request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request body"})
		return
	}

	// 2. Verify the webhook signature. This is a critical security step.
	//    Clerk uses Svix for webhook signing, which uses HMAC-SHA256.
	if !server.verifySvixSignature(body, c.Request.Header) {
		server.logger.Warn("clerk webhook verification failed")
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Webhook verification failed"})
		return
	}

	// 3. Unmarshal the verified payload into our event struct.
	var event clerkUserCreatedEvent
	if err := json.Unmarshal(body, &event); err != nil {
		server.logger.Error("failed to unmarshal clerk webhook payload", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid webhook payload"})
		return
	}

	// 5. Ensure this is a 'user.created' event.
	if event.Type != "user.created" {
		// We can receive other events here, but we choose to only act on user.created.
		// Responding with 200 OK tells Clerk we've received it, even if we didn't act on it.
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Event received but not processed"})
		return
	}

	// 6. Validate the necessary data is present.
	if event.Data.ID == "" || len(event.Data.EmailAddresses) == 0 {
		server.logger.Error("clerk webhook 'user.created' event is missing data", "eventId", event.Type)
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Payload missing required fields"})
		return
	}
	primaryEmail := event.Data.EmailAddresses[0].EmailAddress

	// 7. Delegate user creation to the user service.
	user, err := server.userService.CreateUser(c.Request.Context(), event.Data.ID, primaryEmail)
	if err != nil {
		// The service layer handles logging, so we just need to return the correct HTTP status.
		// The error from the service layer will indicate if it's a conflict or another server error.
		if err == services.ErrUserAlreadyExists {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to create user"})
		}
		return
	}

	// 8. Respond with success.
	server.logger.Info("successfully created user from clerk webhook", "user_id", user.ID)
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": user})
}

/**
 * @description
 * verifySvixSignature verifies the Svix signature of the webhook request.
 * Clerk uses Svix for webhook signing, which uses HMAC-SHA256 with the secret key.
 *
 * @param body The raw request body bytes.
 * @param headers The HTTP request headers.
 * @returns true if the signature is valid, false otherwise.
 */
func (server *Server) verifySvixSignature(body []byte, headers http.Header) bool {
	// Extract the Svix signature from headers
	svixSignature := headers.Get("svix-signature")
	if svixSignature == "" {
		server.logger.Warn("missing svix-signature header")
		return false
	}

	// Extract the timestamp from headers
	svixTimestamp := headers.Get("svix-timestamp")
	if svixTimestamp == "" {
		server.logger.Warn("missing svix-timestamp header")
		return false
	}

	// Get the secret key from config
	secret := server.config.ClerkSecretKey
	if secret == "" {
		server.logger.Error("CLERK_SECRET_KEY is not configured")
		return false
	}

	// Remove the "whsec_" prefix if present (Clerk webhook secrets start with "whsec_")
	secretKey := secret
	if strings.HasPrefix(secret, "whsec_") {
		secretKey = secret[6:] // Remove "whsec_" prefix
	}

	// Decode the secret key from base64
	secretBytes, err := base64.StdEncoding.DecodeString(secretKey)
	if err != nil {
		server.logger.Error("failed to decode secret key", "error", err)
		return false
	}

	// Parse the signature header (format: "v1,signature1 v1,signature2 ...")
	// We need to check each signature version
	signatures := strings.Split(svixSignature, " ")
	for _, sig := range signatures {
		parts := strings.Split(sig, ",")
		if len(parts) != 2 {
			continue
		}

		version := parts[0]
		signature := parts[1]

		// Only verify v1 signatures
		if version != "v1" {
			continue
		}

		// Create the signed content: timestamp + "." + body (as raw bytes)
		// The signed content is the timestamp as string, followed by ".", followed by the raw body bytes
		signedContent := append(append([]byte(svixTimestamp), byte('.')), body...)

		// Compute HMAC-SHA256
		mac := hmac.New(sha256.New, secretBytes)
		mac.Write([]byte(signedContent))
		expectedMAC := mac.Sum(nil)

		// Decode the signature from base64
		receivedMAC, err := base64.StdEncoding.DecodeString(signature)
		if err != nil {
			continue
		}

		// Compare using constant-time comparison
		if hmac.Equal(expectedMAC, receivedMAC) {
			return true
		}
	}

	return false
}

