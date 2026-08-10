package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordHashRejectsWrongPassword(t *testing.T) {
	hash, err := hashPassword("correct password", "test pepper")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash = %q, want argon2id encoding", hash)
	}
	if !verifyPassword("correct password", hash, "test pepper") {
		t.Fatal("correct password did not verify")
	}
	if verifyPassword("wrong password", hash, "test pepper") {
		t.Fatal("wrong password verified")
	}
}

func TestAuthenticateRejectsExpiredToken(t *testing.T) {
	a := &app{jwtSecret: []byte("a test secret longer than thirty-two bytes")}
	header := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := b64url([]byte(`{"uid":1,"username":"tester","iat":1,"exp":1}`))
	input := header + "." + payload
	mac := hmac.New(sha256.New, a.jwtSecret)
	_, _ = mac.Write([]byte(input))
	token := input + "." + b64url(mac.Sum(nil))
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if _, err := a.authenticate(req); err == nil {
		t.Fatal("expired token verified")
	}
}

func TestValidSchoolID(t *testing.T) {
	for _, id := range []string{"0", "thu-cs_2027", "school.42"} {
		if !validSchoolID(id) {
			t.Fatalf("validSchoolID(%q) = false", id)
		}
	}
	for _, id := range []string{"", "a/b", "a b"} {
		if validSchoolID(id) {
			t.Fatalf("validSchoolID(%q) = true", id)
		}
	}
}
