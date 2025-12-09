package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	appleJWKSURL = "https://appleid.apple.com/auth/keys"
	appleIssuer  = "https://appleid.apple.com"
)

// AppleJWKS represents Apple's JSON Web Key Set
type AppleJWKS struct {
	Keys []AppleJWK `json:"keys"`
}

// AppleJWK represents a single JSON Web Key
type AppleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// AppleIdentityTokenClaims represents the claims in an Apple identity token
type AppleIdentityTokenClaims struct {
	jwt.RegisteredClaims
	Email    string `json:"email,omitempty"`
	EmailVerified bool `json:"email_verified,omitempty"`
	IsPrivateEmail bool `json:"is_private_email,omitempty"`
}

// ValidateAppleIdentityToken validates an Apple identity token and returns the user's sub (stable ID)
func ValidateAppleIdentityToken(identityToken string, clientID string) (string, error) {
	// Parse the token without verification first to get the key ID
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(identityToken, jwt.MapClaims{})

	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	// Get the key ID from the token header
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid kid in token header")
	}

	// Fetch Apple's public keys
	keys, err := fetchApplePublicKeys()
	if err != nil {
		return "", fmt.Errorf("failed to fetch Apple public keys: %w", err)
	}

	// Find the matching key
	var publicKey *rsa.PublicKey
	for _, key := range keys.Keys {
		if key.Kid == kid {
			publicKey, err = convertJWKToRSAPublicKey(key)
			if err != nil {
				return "", fmt.Errorf("failed to convert JWK to RSA public key: %w", err)
			}
			break
		}
	}

	if publicKey == nil {
		return "", fmt.Errorf("no matching public key found for kid: %s", kid)
	}

	// Parse and validate the token with the public key
	claims := &AppleIdentityTokenClaims{}
	validToken, err := jwt.ParseWithClaims(identityToken, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to validate token: %w", err)
	}

	if !validToken.Valid {
		return "", fmt.Errorf("token is not valid")
	}

	// Verify issuer
	if claims.Issuer != appleIssuer {
		return "", fmt.Errorf("invalid issuer: %s", claims.Issuer)
	}

	// Verify audience (client ID)
	// Audience can be a string or array in JWT v5
	audience := ""
	if claims.Audience != nil {
		if len(claims.Audience) > 0 {
			audience = claims.Audience[0]
		}
	}
	if audience == "" {
		return "", fmt.Errorf("missing audience claim in token")
	}
	if audience != clientID {
		return "", fmt.Errorf("invalid audience: %s", audience)
	}

	// Verify expiration
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return "", fmt.Errorf("token has expired")
	}

	// Return the sub (stable Apple user ID)
	if claims.Subject == "" {
		return "", fmt.Errorf("missing sub claim in token")
	}

	return claims.Subject, nil
}

// fetchApplePublicKeys fetches Apple's public keys from their JWKS endpoint
func fetchApplePublicKeys() (*AppleJWKS, error) {
	resp, err := http.Get(appleJWKSURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Apple JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Apple JWKS endpoint returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS response: %w", err)
	}

	var jwks AppleJWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	return &jwks, nil
}

// convertJWKToRSAPublicKey converts an Apple JWK to an RSA public key
func convertJWKToRSAPublicKey(jwk AppleJWK) (*rsa.PublicKey, error) {
	// Decode the modulus (n)
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	// Decode the exponent (e)
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	// Convert exponent bytes to int
	var eInt int
	for _, b := range eBytes {
		eInt = eInt<<8 | int(b)
	}

	// Create RSA public key
	publicKey := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: eInt,
	}

	return publicKey, nil
}

