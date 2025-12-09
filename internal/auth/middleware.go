package auth

import (
	"net/http"
	"strings"
)

// JWTMiddleware creates HTTP middleware that validates JWT tokens
func JWTMiddleware(jwtSecret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Check for Bearer prefix
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		// Validate token
		_, _, err := ValidateSessionToken(tokenString, jwtSecret)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Token is valid, proceed to next handler
		next.ServeHTTP(w, r)
	})
}

