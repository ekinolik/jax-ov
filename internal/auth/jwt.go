package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// SessionClaims represents the claims in our session JWT
type SessionClaims struct {
	jwt.RegisteredClaims
	SessionID string `json:"session_id"`
}

// CreateSessionToken creates a JWT session token for an authenticated user
func CreateSessionToken(sub string, secret string, expiryDuration time.Duration) (string, error) {
	// Generate a unique session ID
	sessionID := uuid.New().String()

	// Create claims
	now := time.Now()
	claims := &SessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiryDuration)),
		},
		SessionID: sessionID,
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateSessionToken validates a session JWT token and returns the user's sub and session ID
func ValidateSessionToken(tokenString string, secret string) (string, string, error) {
	// Parse and validate the token
	claims := &SessionClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return "", "", fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return "", "", fmt.Errorf("token is not valid")
	}

	// Verify expiration
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return "", "", fmt.Errorf("token has expired")
	}

	// Return sub and session ID
	if claims.Subject == "" {
		return "", "", fmt.Errorf("missing sub claim in token")
	}

	if claims.SessionID == "" {
		return "", "", fmt.Errorf("missing session_id claim in token")
	}

	return claims.Subject, claims.SessionID, nil
}

