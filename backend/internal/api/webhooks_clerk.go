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
		ID                   string `json:"id"`
		PrimaryEmailAddressID string `json:"primary_email_address_id"`
		EmailAddresses       []struct {
			EmailAddress string `json:"email_address"`
			ID           string `json:"id"`
		} `json:"email_addresses"`
		PhoneNumbers []struct {
			PhoneNumber string `json:"phone_number"`
			ID          string `json:"id"`
		} `json:"phone_numbers"`
		PrimaryPhoneNumberID string `json:"primary_phone_number_id"`
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
		server.logger.Error("failed to unmarshal clerk webhook payload", "error", err, "body", string(body))
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid webhook payload"})
		return
	}
	
	server.logger.Debug("parsed webhook event", 
		"type", event.Type,
		"user_id", event.Data.ID,
		"email_count", len(event.Data.EmailAddresses))

	// 5. Ensure this is a 'user.created' event.
	if event.Type != "user.created" {
		// We can receive other events here, but we choose to only act on user.created.
		// Responding with 200 OK tells Clerk we've received it, even if we didn't act on it.
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Event received but not processed"})
		return
	}

	// 6. Validate the necessary data is present.
	if event.Data.ID == "" {
		server.logger.Error("clerk webhook 'user.created' event is missing user ID", 
			"eventId", event.Type,
			"event_data", string(body))
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Payload missing user ID"})
		return
	}
	
	// Extract email address - check email_addresses array first, then fallback to phone number
	var primaryEmail string
	
	// First, try to find email from email_addresses array
	for _, email := range event.Data.EmailAddresses {
		if email.EmailAddress != "" {
			primaryEmail = email.EmailAddress
			break
		}
	}
	
	// If no email found, create a placeholder email using the user ID
	// This handles cases where:
	// 1. User signs up with phone only (email_addresses is empty)
	// 2. User is created before email verification (email_addresses is empty but primary_email_address_id exists)
	// 3. User has no email or phone at all (we still need to create the user record)
	if primaryEmail == "" {
		// Create a placeholder email using the user ID
		// Format: user_{clerk_id}@clerk.placeholder
		// This ensures we can create the user in the database even if no email is provided yet
		primaryEmail = event.Data.ID + "@clerk.placeholder"
		
		// Try to find primary phone number for logging
		var phoneNumber string
		if event.Data.PrimaryPhoneNumberID != "" {
			for _, phone := range event.Data.PhoneNumbers {
				if phone.ID == event.Data.PrimaryPhoneNumberID && phone.PhoneNumber != "" {
					phoneNumber = phone.PhoneNumber
					break
				}
			}
		}
		
		server.logger.Warn("user created without email in payload, using placeholder", 
			"user_id", event.Data.ID,
			"primary_email_address_id", event.Data.PrimaryEmailAddressID,
			"phone", phoneNumber,
			"placeholder_email", primaryEmail,
			"note", "Email may be added later via user.updated webhook")
	}

	// 7. Delegate user creation to the user service.
	// According to Clerk docs, webhooks should return 2xx status codes for success.
	// When a user already exists (idempotent operation), we should return 200 OK
	// to acknowledge the webhook as successful and prevent retries.
	user, err := server.userService.CreateUser(c.Request.Context(), event.Data.ID, primaryEmail)
	if err != nil {
		// If user already exists, this is an idempotent operation - treat as success
		// According to Clerk docs: Return 2xx status to acknowledge webhook and prevent retries
		if err == services.ErrUserAlreadyExists {
			// Try to fetch the existing user to return in the response
			existingUser, findErr := server.userService.GetUserByClerkID(c.Request.Context(), event.Data.ID)
			if findErr != nil {
				// Even if we can't fetch the user, we still acknowledge the webhook as successful
				// because the unique constraint violation confirms the user exists
				// This prevents Clerk from retrying the webhook
				server.logger.Warn("user already exists but could not fetch user details", 
					"clerk_id", event.Data.ID, 
					"error", findErr)
				// Return 200 OK to acknowledge the webhook as successful
				// The user exists (confirmed by unique constraint), so we treat this as success
				c.JSON(http.StatusOK, gin.H{
					"status": "success", 
					"message": "User already exists (idempotent operation)",
					"clerk_id": event.Data.ID,
				})
				return
			}
			server.logger.Info("webhook acknowledged - user already exists (idempotent)", 
				"user_id", existingUser.ID, 
				"clerk_id", event.Data.ID)
			// Return 200 OK to acknowledge the webhook as successful
			// This prevents Clerk from retrying the webhook
			c.JSON(http.StatusOK, gin.H{
				"status": "success", 
				"message": "User already exists", 
				"data": existingUser,
			})
			return
		}
		
		// For other errors, return 500 to indicate server error
		// Clerk will retry these errors according to their retry schedule
		server.logger.Error("failed to create user from webhook", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to create user"})
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

	// Extract the Svix ID from headers (required for signed content)
	svixID := headers.Get("svix-id")
	if svixID == "" {
		server.logger.Warn("missing svix-id header")
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

	// Svix secret format: The secret starts with "whsec_" and the rest is base64 encoded
	// According to Svix documentation, we need to use the secret as-is but decode only the part after "whsec_"
	var secretBytes []byte
	var err error
	
	if strings.HasPrefix(secret, "whsec_") {
		// Extract the base64 part after "whsec_"
		secretKey := secret[6:] // Remove "whsec_" prefix
		secretBytes, err = base64.StdEncoding.DecodeString(secretKey)
		if err != nil {
			server.logger.Error("failed to decode secret key from whsec format", "error", err)
			return false
		}
	} else {
		// If no prefix, try to decode the entire secret as base64
		secretBytes, err = base64.StdEncoding.DecodeString(secret)
		if err != nil {
			server.logger.Error("failed to decode secret key", "error", err)
			return false
		}
	}

	// Parse the signature header
	// Svix signature format: "v1,signature1 v1,signature2 ..." (space-separated)
	// Each signature is "version,base64_signature"
	signatures := strings.Split(svixSignature, " ")
	
	for _, sig := range signatures {
		parts := strings.Split(sig, ",")
		if len(parts) != 2 {
			server.logger.Debug("invalid signature format", "signature", sig)
			continue
		}

		version := parts[0]
		signatureStr := parts[1]

		// Only verify v1 signatures
		if version != "v1" {
			server.logger.Debug("skipping non-v1 signature", "version", version)
			continue
		}

		// Create the signed content according to Svix spec:
		// The signed content is: svix_id + "." + svix_timestamp + "." + body
		// Format: "svix_id.svix_timestamp.body"
		idBytes := []byte(svixID)
		timestampBytes := []byte(svixTimestamp)
		signedContent := make([]byte, 0, len(idBytes)+1+len(timestampBytes)+1+len(body))
		signedContent = append(signedContent, idBytes...)
		signedContent = append(signedContent, byte('.'))
		signedContent = append(signedContent, timestampBytes...)
		signedContent = append(signedContent, byte('.'))
		signedContent = append(signedContent, body...)

		// Compute HMAC-SHA256
		mac := hmac.New(sha256.New, secretBytes)
		mac.Write(signedContent)
		expectedMAC := mac.Sum(nil)

		// Decode the signature from base64
		receivedMAC, err := base64.StdEncoding.DecodeString(signatureStr)
		if err != nil {
			server.logger.Debug("failed to decode signature", "error", err, "signature", signatureStr)
			continue
		}

		// Compare using constant-time comparison
		if hmac.Equal(expectedMAC, receivedMAC) {
			server.logger.Debug("signature verification successful")
			return true
		}
	}

	server.logger.Warn("signature verification failed for all signatures", 
		"signature_header", svixSignature, 
		"svix_id", svixID,
		"timestamp", svixTimestamp,
		"body_length", len(body))
	return false
}

