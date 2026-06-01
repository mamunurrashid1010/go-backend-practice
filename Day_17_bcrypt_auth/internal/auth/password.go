package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of the plaintext password. The hash
// string embeds the version, cost, and a random salt, so it's all you need
// to store and later verify.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
// bcrypt.CompareHashAndPassword is constant-time, so it doesn't leak how
// many characters matched via timing. Never compare hashes with ==.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
