/**
 * @description
 * This file contains the HTTP handlers for vault-related API endpoints.
 * It allows the remote signer service to retrieve private keys using secret references.
 *
 * Key features:
 * - Secure Access: Only allows retrieval of private keys via secret references
 * - Internal Service: Intended for use by the remote signer service
 * - Authentication: Should be protected by service-to-service authentication in production
 */

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

/**
 * @description
 * getPrivateKey is a Gin handler that retrieves a private key from the vault
 * using a secret reference. This endpoint is used by the remote signer service.
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - In production, this endpoint should be protected by service-to-service authentication
 * - The secret reference is passed as a URL parameter
 */
func (server *Server) getPrivateKey(c *gin.Context) {
	// Get the secret reference from the URL parameter
	secretRef := c.Param("secretRef")
	if secretRef == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "secret reference is required",
		})
		return
	}

	// TODO: In production, add service-to-service authentication here
	// For example, check for an API key or JWT token in the request headers
	// For now, we'll allow access (this should be restricted in production)

	// Retrieve the private key from the vault service
	privateKey, err := server.vaultService.GetPrivateKey(c.Request.Context(), secretRef)
	if err != nil {
		server.logger.Error("failed to retrieve private key from vault", "error", err, "secret_ref", secretRef)
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "private key not found",
		})
		return
	}

	// Return the private key (in production, ensure this is over HTTPS and authenticated)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"private_key": privateKey,
		},
	})
}

