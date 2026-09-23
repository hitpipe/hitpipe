package session

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AccessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func GenerateAccessToken(sid, sub string, secret []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, AccessClaims{
		SessionID: sid,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "DukeD",
			Subject:   sub,
			Audience:  jwt.ClaimStrings{"web"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			NotBefore: nil,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        "",
		},
	})

	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %v", err)
	}

	return tokenString, nil
}
