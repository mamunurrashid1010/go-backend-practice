package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AccessTokenClaims struct {
	UserID int64  `json:"uid"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

func NewTokenIssuer(secret string, ttl time.Duration, issuer string) *TokenIssuer {
	if secret == "" {
		panic("auth: JWT_SECRET is empty")
	}
	return &TokenIssuer{secret: []byte(secret), ttl: ttl, issuer: issuer}
}

func (i *TokenIssuer) Issue(u User) (string, time.Duration, error) {
	now := time.Now().UTC()
	claims := AccessTokenClaims{
		UserID: u.ID, Email: u.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: i.issuer, Subject: strconv.FormatInt(u.ID, 10),
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
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

type TokenVerifier struct {
	secret []byte
	issuer string
}

func NewTokenVerifier(secret, issuer string) *TokenVerifier {
	if secret == "" {
		panic("auth: JWT_SECRET is empty")
	}
	return &TokenVerifier{secret: []byte(secret), issuer: issuer}
}

func (v *TokenVerifier) Verify(tokenStr string) (*AccessTokenClaims, error) {
	var claims AccessTokenClaims
	parsed, err := jwt.ParseWithClaims(tokenStr, &claims,
		func(t *jwt.Token) (any, error) { return v.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return &claims, nil
}
