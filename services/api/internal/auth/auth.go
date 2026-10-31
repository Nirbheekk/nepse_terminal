// Package auth implements credential hashing, JWT access tokens and opaque
// refresh/CSRF secrets.
//
// Design notes:
//   - Passwords use Argon2id (memory-hard) with a per-password random salt.
//   - Access tokens are short-lived JWTs held only in HttpOnly cookies, so a
//     script cannot read them.
//   - Refresh tokens are opaque random values. Only their SHA-256 digest is
//     stored, so a database leak does not yield usable tokens.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP-recommended profile for interactive logins).
const (
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonTime    uint32 = 2
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

var (
	ErrInvalidHash     = errors.New("invalid password hash format")
	ErrInvalidToken    = errors.New("invalid token")
	ErrIncompatibleAlg = errors.New("unexpected signing method")
)

// HashPassword returns an encoded Argon2id hash: $argon2id$v=19$m=...,t=...,p=...$salt$hash
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword compares a candidate password against an encoded hash in
// constant time.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidHash
	}
	if version != argon2.Version {
		return false, ErrInvalidHash
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidHash
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// Manager issues and validates access tokens.
type Manager struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

// Claims carried by an access token. Roles are embedded so authorisation does
// not require a database round trip on every request.
type Claims struct {
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

func NewManager(secret []byte, issuer string, ttl time.Duration) *Manager {
	return &Manager{secret: secret, issuer: issuer, ttl: ttl}
}

// TTL is the access-token lifetime.
func (m *Manager) TTL() time.Duration { return m.ttl }

// Issue creates a signed HS256 access token.
func (m *Manager) Issue(userID string, roles []string) (string, time.Time, error) {
	now := time.Now().UTC()
	expires := now.Add(m.ttl)
	claims := Claims{
		Roles: roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

// Parse validates signature, algorithm, issuer and expiry.
func (m *Manager) Parse(raw string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, ErrIncompatibleAlg
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(m.issuer))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// NewOpaqueToken returns a URL-safe random secret and its SHA-256 digest.
func NewOpaqueToken() (raw string, digest []byte, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashSecret(raw), nil
}

// HashSecret is the deterministic digest stored for opaque tokens.
func HashSecret(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// RandomHex returns n random bytes hex-encoded, used for CSRF tokens and ids.
func RandomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, b := range buf {
		sb.WriteString(strconv.FormatUint(uint64(b)>>4, 16))
		sb.WriteString(strconv.FormatUint(uint64(b)&0x0f, 16))
	}
	return sb.String(), nil
}
