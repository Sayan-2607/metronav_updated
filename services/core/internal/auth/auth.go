// Package auth provides password hashing (PBKDF2-HMAC-SHA256, per RFC 8018),
// HS256 JSON Web Tokens (RFC 7519) and HMAC-signed ticket tokens, using only
// the Go standard library.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Iterations follows the OWASP Password Storage Cheat Sheet recommendation
// for PBKDF2-HMAC-SHA256 (600,000).
const Iterations = 600_000

func pbkdf2(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hLen := prf.Size()
	blocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	buf := make([]byte, 4)
	for b := 1; b <= blocks; b++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(b))
		prf.Write(buf)
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

// HashPassword returns "pbkdf2-sha256$iter$salt$hash" (base64 raw std).
func HashPassword(pw string) (string, error) {
	return hashWithIter(pw, Iterations)
}

func hashWithIter(pw string, iter int) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2([]byte(pw), salt, iter, 32)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iter, enc.EncodeToString(salt), enc.EncodeToString(dk)), nil
}

func CheckPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got := pbkdf2([]byte(pw), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ---- JWT (HS256) ----

type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Role  string `json:"role"`
	Exp   int64  `json:"exp"`
	Iat   int64  `json:"iat"`
}

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

var b64 = base64.RawURLEncoding

func sign(secret, msg []byte) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write(msg)
	return m.Sum(nil)
}

func IssueJWT(secret string, c Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	c.Iat, c.Exp = now.Unix(), now.Add(ttl).Unix()
	h := b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	p, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	unsigned := h + "." + b64.EncodeToString(p)
	return unsigned + "." + b64.EncodeToString(sign([]byte(secret), []byte(unsigned))), nil
}

func ParseJWT(secret, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	var hdr struct{ Alg string `json:"alg"` }
	hb, err := b64.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &hdr) != nil || hdr.Alg != "HS256" {
		return nil, ErrInvalidToken // rejects alg=none and algorithm confusion
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, sign([]byte(secret), []byte(parts[0]+"."+parts[1]))) {
		return nil, ErrInvalidToken
	}
	pb, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var c Claims
	if err := json.Unmarshal(pb, &c); err != nil {
		return nil, ErrInvalidToken
	}
	if time.Now().Unix() >= c.Exp {
		return nil, ErrExpiredToken
	}
	return &c, nil
}

// ---- Ticket tokens: "MN1.<ticketID>.<expUnix>.<sig>" ----

func SignTicket(secret, ticketID string, exp time.Time) string {
	body := fmt.Sprintf("MN1.%s.%d", ticketID, exp.Unix())
	return body + "." + b64.EncodeToString(sign([]byte(secret), []byte(body)))[:22]
}

func VerifyTicket(secret, token string) (ticketID string, exp time.Time, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 4 || parts[0] != "MN1" {
		return "", time.Time{}, ErrInvalidToken
	}
	body := strings.Join(parts[:3], ".")
	want := b64.EncodeToString(sign([]byte(secret), []byte(body)))[:22]
	if !hmac.Equal([]byte(want), []byte(parts[3])) {
		return "", time.Time{}, ErrInvalidToken
	}
	e, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", time.Time{}, ErrInvalidToken
	}
	return parts[1], time.Unix(e, 0), nil
}
