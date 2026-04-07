package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const sessionTTL = 7 * 24 * time.Hour

// Claims matches Node jsonwebtoken payload { sub: userId }.
type Claims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}

func jwtSecret(cfgSecret string, nodeEnv string) (string, error) {
	s := strings.TrimSpace(cfgSecret)
	if s != "" {
		return s, nil
	}
	if nodeEnv == "development" || nodeEnv == "dev" {
		return "dev-only-insecure-jwt-secret-change-for-production", nil
	}
	return "", errors.New("DM_JWT_SECRET is required in production")
}

// SignSessionToken issues a JWT for Mongo user id string.
func SignSessionToken(userIDHex string, cfgSecret string, nodeEnv string) (string, error) {
	sec, err := jwtSecret(cfgSecret, nodeEnv)
	if err != nil {
		return "", err
	}
	if userIDHex == "" {
		return "", errors.New("signSessionToken: invalid user")
	}
	now := time.Now()
	claims := Claims{
		Sub: userIDHex,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(sessionTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(sec))
}

// VerifySessionToken returns sub or empty string.
func VerifySessionToken(token string, cfgSecret string, nodeEnv string) (sub string, err error) {
	sec, err := jwtSecret(cfgSecret, nodeEnv)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token) == "" {
		return "", nil
	}
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(sec), nil
	})
	if err != nil || !parsed.Valid {
		return "", nil
	}
	c, ok := parsed.Claims.(*Claims)
	if !ok || c.Sub == "" {
		return "", nil
	}
	return c.Sub, nil
}
