package cryptoutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestAuthorizationCodeCipherEncryptDecrypt(t *testing.T) {
	cipher := newTestCipher(t)

	encrypted, err := cipher.Encrypt("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if encrypted.Ciphertext == "" || encrypted.Salt == "" {
		t.Fatal("missing encrypted fields")
	}
	if encrypted.Ciphertext == "12345678901234567890123456789012" {
		t.Fatal("ciphertext equals plaintext")
	}

	got, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != "12345678901234567890123456789012" {
		t.Fatalf("Decrypt() = %q", got)
	}
}

func TestAuthorizationCodeCipherUsesFreshSalt(t *testing.T) {
	cipher := newTestCipher(t)

	first, err := cipher.Encrypt("same-code")
	if err != nil {
		t.Fatalf("Encrypt(first) error = %v", err)
	}
	second, err := cipher.Encrypt("same-code")
	if err != nil {
		t.Fatalf("Encrypt(second) error = %v", err)
	}
	if first.Salt == second.Salt {
		t.Fatal("expected fresh salt")
	}
	if first.Ciphertext == second.Ciphertext {
		t.Fatal("expected different ciphertext")
	}
}

func newTestCipher(t *testing.T) *AuthorizationCodeCipher {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	cipher, err := NewAuthorizationCodeCipher(string(pemBytes))
	if err != nil {
		t.Fatalf("NewAuthorizationCodeCipher() error = %v", err)
	}
	return cipher
}
