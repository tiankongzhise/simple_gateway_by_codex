package cryptoutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

const AuthorizationCodeAlgorithm = "RSA-OAEP-SHA256+salt-v1"

// AuthorizationCodeCipher encrypts and decrypts permanent authorization codes.
type AuthorizationCodeCipher struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

// EncryptedAuthorizationCode is the database-safe encrypted form.
type EncryptedAuthorizationCode struct {
	Ciphertext string
	Salt       string
	Algorithm  string
}

// NewAuthorizationCodeCipher parses a PEM RSA private key and derives its public key.
func NewAuthorizationCodeCipher(privateKeyPEM string) (*AuthorizationCodeCipher, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(privateKeyPEM)))
	if block == nil {
		return nil, errors.New("RSA private key PEM is invalid")
	}

	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = parsed
	} else if parsedAny, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := parsedAny.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("PKCS8 private key is not RSA")
		}
		key = rsaKey
	} else {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}

	return &AuthorizationCodeCipher{privateKey: key, publicKey: &key.PublicKey}, nil
}

// Encrypt encrypts authorizationCode with a fresh random salt.
func (c *AuthorizationCodeCipher) Encrypt(authorizationCode string) (EncryptedAuthorizationCode, error) {
	if strings.TrimSpace(authorizationCode) == "" {
		return EncryptedAuthorizationCode{}, errors.New("authorization code is required")
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return EncryptedAuthorizationCode{}, fmt.Errorf("generate salt: %w", err)
	}

	saltEncoded := base64.RawStdEncoding.EncodeToString(salt)
	plaintext := []byte(saltEncoded + ":" + authorizationCode)
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, c.publicKey, plaintext, []byte(AuthorizationCodeAlgorithm))
	if err != nil {
		return EncryptedAuthorizationCode{}, fmt.Errorf("encrypt authorization code: %w", err)
	}

	return EncryptedAuthorizationCode{
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
		Salt:       saltEncoded,
		Algorithm:  AuthorizationCodeAlgorithm,
	}, nil
}

// Decrypt decrypts and validates the encrypted authorization code.
func (c *AuthorizationCodeCipher) Decrypt(value EncryptedAuthorizationCode) (string, error) {
	if value.Algorithm != AuthorizationCodeAlgorithm {
		return "", fmt.Errorf("unsupported authorization code algorithm: %s", value.Algorithm)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(value.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, c.privateKey, ciphertext, []byte(AuthorizationCodeAlgorithm))
	if err != nil {
		return "", fmt.Errorf("decrypt authorization code: %w", err)
	}

	salt, code, ok := strings.Cut(string(plaintext), ":")
	if !ok {
		return "", errors.New("encrypted authorization code payload is invalid")
	}
	if salt != value.Salt {
		return "", errors.New("authorization code salt mismatch")
	}
	if code == "" {
		return "", errors.New("authorization code payload is empty")
	}
	return code, nil
}

// DecryptBinding decrypts a persisted service group binding secret.
func (c *AuthorizationCodeCipher) DecryptBinding(ciphertext, salt, algorithm string) (string, error) {
	return c.Decrypt(EncryptedAuthorizationCode{
		Ciphertext: ciphertext,
		Salt:       salt,
		Algorithm:  algorithm,
	})
}
