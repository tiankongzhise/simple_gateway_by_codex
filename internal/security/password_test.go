package security

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("secret-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == "secret-password" {
		t.Fatal("password was not hashed")
	}
	if !CheckPassword(hash, "secret-password") {
		t.Fatal("expected password to match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestNewTokenAndHashToken(t *testing.T) {
	token, err := NewToken(32)
	if err != nil {
		t.Fatalf("NewToken() error = %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if HashToken(token) == HashToken(token+"x") {
		t.Fatal("hash collision for different input")
	}
}
