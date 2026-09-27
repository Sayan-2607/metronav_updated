package auth

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// Test vector for PBKDF2-HMAC-SHA256 (RFC 7914 §11, c=1, dkLen=64).
func TestPBKDF2Vector(t *testing.T) {
	got := hex.EncodeToString(pbkdf2([]byte("passwd"), []byte("salt"), 1, 64))
	want := "55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc49ca9cccf179b645991664b39d77ef317c71b845b1e30bd509112041d3a19783"
	if got != want {
		t.Fatalf("pbkdf2 mismatch\n got %s\nwant %s", got, want)
	}
}

func TestPasswordRoundTrip(t *testing.T) {
	h, err := hashWithIter("s3cret-pass", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword("s3cret-pass", h) || CheckPassword("wrong", h) {
		t.Fatal("password check failed")
	}
}

func TestJWT(t *testing.T) {
	tok, err := IssueJWT("k", Claims{Sub: "u1", Role: "ADMIN"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseJWT("k", tok)
	if err != nil || c.Sub != "u1" || c.Role != "ADMIN" {
		t.Fatalf("parse failed: %v %+v", err, c)
	}
	if _, err := ParseJWT("other", tok); err != ErrInvalidToken {
		t.Fatal("wrong secret accepted")
	}
	parts := strings.Split(tok, ".")
	if _, err := ParseJWT("k", "eyJhbGciOiJub25lIn0."+parts[1]+"."); err != ErrInvalidToken {
		t.Fatal("alg=none accepted")
	}
	exp, _ := IssueJWT("k", Claims{Sub: "u1"}, -time.Second)
	if _, err := ParseJWT("k", exp); err != ErrExpiredToken {
		t.Fatalf("expired token accepted: %v", err)
	}
}

func TestTicketToken(t *testing.T) {
	tok := SignTicket("s", "TKT-1", time.Now().Add(time.Hour))
	id, _, err := VerifyTicket("s", tok)
	if err != nil || id != "TKT-1" {
		t.Fatal("ticket verify failed")
	}
	if _, _, err := VerifyTicket("s", strings.Replace(tok, "TKT-1", "TKT-2", 1)); err == nil {
		t.Fatal("tampered ticket accepted")
	}
}
