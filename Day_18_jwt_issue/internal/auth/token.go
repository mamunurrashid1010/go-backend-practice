package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessTokenClaims embeds the standard registered claims and adds the
// custom ones our app cares about.
//
// Keep custom claims small and non-sensitive — the payload is base64-encoded,
// not encrypted. Anyone holding the token can read it.
type AccessTokenClaims struct {
	UserID int64  `json:"uid"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// TokenIssuer signs access tokens.
//
// One instance lives in main(). The secret, TTL, and issuer come from config.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

// NewTokenIssuer panics on empty secret — that's a fatal misconfiguration,
// not something to defensively recover from at runtime.
func NewTokenIssuer(secret string, ttl time.Duration, issuer string) *TokenIssuer {
	if secret == "" {
		panic("auth: JWT_SECRET is empty")
	}
	return &TokenIssuer{
		secret: []byte(secret),
		ttl:    ttl,
		issuer: issuer,
	}
}

// Issue signs an access token for the given user. Returns the signed string,
// the configured TTL (handy for the response's "expires_in" field), and any
// signing error.
func (i *TokenIssuer) Issue(u User) (string, time.Duration, error) {
	now := time.Now().UTC()

	claims := AccessTokenClaims{
		UserID: u.ID,
		Email:  u.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   strconv.FormatInt(u.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
	}

	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(i.secret)
	if err != nil {
		return "", 0, fmt.Errorf("sign jwt: %w", err)
	}
	return signed, i.ttl, nil
}
