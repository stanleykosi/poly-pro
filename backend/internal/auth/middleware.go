/**
 * @description
 * This file contains the authentication middleware for the Gin server.
 * It is responsible for validating JSON Web Tokens (JWTs) issued by Clerk
 * for protected API routes.
 *
 * Key features:
 * - JWT Validation: Verifies the signature and claims of the token.
 * - JWKS Integration: Uses a JWKS (JSON Web Key Set) client to fetch Clerk's
 *   public keys for signature verification. The key set is cached to avoid
 *   excessive network requests.
 * - Context Injection: Upon successful validation, the user's Clerk ID (from the 'sub' claim)
 *   is injected into the Gin context for use by downstream handlers.
 * - Error Handling: Returns a 401 Unauthorized status with a clear error message
 *   if authentication fails for any reason (e.g., missing token, invalid signature,
 *   expired token).
 *
 * @dependencies
 * - github.com/gin-gonic/gin: The web framework.
 * - github.com/golang-jwt/jwt/v5: For parsing and validating JWTs.
 * - github.com/MicahParks/keyfunc/v2: For fetching and managing the JWKS.
 */

package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// GinContextKey is a custom type to avoid key collisions in the Gin context.
type GinContextKey string

const (
	// ClerkUserIDKey is the key used to store the authenticated user's Clerk ID in the Gin context.
	ClerkUserIDKey GinContextKey = "clerkUserID"
)

/**
 * @description
 * NewAuthMiddleware creates a Gin middleware that validates Clerk JWTs.
 *
 * @param clerkIssuerURL The URL of the Clerk instance (e.g., "https://clerk.your-domain.com").
 *        This is used to construct the JWKS URL.
 * @returns A gin.HandlerFunc that can be used as middleware.
 * @returns An error if the JWKS key set cannot be initialized.
 *
 * @notes
 * - The function initializes a JWKS client which automatically handles fetching, caching,
 *   and refreshing Clerk's public keys.
 */
func NewAuthMiddleware(clerkIssuerURL string) (gin.HandlerFunc, error) {
	// Construct the JWKS URL from the issuer URL provided by Clerk.
	// This follows the OpenID Connect discovery standard.
	jwksURL := strings.TrimSuffix(clerkIssuerURL, "/") + "/.well-known/jwks.json"

	// Create a new JWKS key set from the URL.
	// keyfunc handles caching and periodic refreshes of the keys.
	jwks, err := keyfunc.Get(jwksURL, keyfunc.Options{
		Ctx:            context.Background(),
		RefreshTimeout: time.Hour,
		// Optional: configure error handling for refresh failures.
	})
	if err != nil {
		return nil, err
	}

	// Return the middleware handler function.
	return func(c *gin.Context) {
		// 1. Get the token from the Authorization header.
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Authorization header is required"})
			return
		}

		// 2. Check if the header is in the format "Bearer <token>".
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Authorization header format must be Bearer {token}"})
			return
		}
		tokenString := parts[1]

		// 3. Parse and validate the token.
		// The keyfunc from the JWKS client is used to find the correct public key
		// based on the 'kid' (Key ID) in the JWT header.
		token, err := jwt.Parse(tokenString, jwks.Keyfunc)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid token: " + err.Error()})
			return
		}

		// 4. Check if the token is valid (signature and expiration verified).
		if !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Token is invalid"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Failed to parse token claims"})
			return
		}

		// 4. Verify the issuer claim matches our expected Clerk issuer URL.
		// This ensures the token was issued by the correct Clerk instance.
		expectedIssuer := strings.TrimSuffix(clerkIssuerURL, "/")
		issuer, ok := claims["iss"].(string)
		if !ok || issuer != expectedIssuer {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Token issuer does not match expected issuer"})
			return
		}

		// 5. Extract the subject ('sub' claim), which is the Clerk User ID.
		clerkUserID, ok := claims["sub"].(string)
		if !ok || clerkUserID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Subject (sub) claim is missing or invalid in token"})
			return
		}

		// 6. Set the Clerk User ID in the Gin context for downstream handlers.
		c.Set(string(ClerkUserIDKey), clerkUserID)

		// 7. Proceed to the next handler in the chain.
		c.Next()
	}, nil
}

