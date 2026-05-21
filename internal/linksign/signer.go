package linksign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ExpiresParam   = "gw_expires"
	SignatureParam = "gw_signature"
	keyContext     = "simple-gateway:signed-link:v1"
)

var (
	ErrMissingSignature = errors.New("missing signed link parameters")
	ErrInvalidExpires   = errors.New("invalid signed link expires")
	ErrExpired          = errors.New("signed link expired")
	ErrInvalidSignature = errors.New("invalid signed link signature")
)

type Signer struct {
	key []byte
}

func New(secret string) Signer {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(keyContext))
	return Signer{key: mac.Sum(nil)}
}

func (s Signer) Sign(method, path, rawQuery string, expiresAt time.Time) string {
	expires := strconv.FormatInt(expiresAt.Unix(), 10)
	payload := signingPayload(method, path, canonicalBusinessQuery(rawQuery), expires)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s Signer) AddSignature(method, path, rawQuery string, expiresAt time.Time) string {
	values, _ := url.ParseQuery(rawQuery)
	values.Del(ExpiresParam)
	values.Del(SignatureParam)
	expires := strconv.FormatInt(expiresAt.Unix(), 10)
	values.Set(ExpiresParam, expires)
	values.Set(SignatureParam, s.Sign(method, path, values.Encode(), expiresAt))
	return values.Encode()
}

func (s Signer) Verify(method, path, rawQuery string, now time.Time) (string, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", ErrInvalidSignature
	}
	expires := strings.TrimSpace(values.Get(ExpiresParam))
	signature := strings.TrimSpace(values.Get(SignatureParam))
	if expires == "" || signature == "" {
		return "", ErrMissingSignature
	}
	expiresUnix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || expiresUnix <= 0 {
		return "", ErrInvalidExpires
	}
	if !now.Before(time.Unix(expiresUnix, 0)) {
		return "", ErrExpired
	}
	values.Del(ExpiresParam)
	values.Del(SignatureParam)
	businessQuery := values.Encode()
	expected := s.Sign(method, path, businessQuery, time.Unix(expiresUnix, 0))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return "", ErrInvalidSignature
	}
	return businessQuery, nil
}

func canonicalBusinessQuery(rawQuery string) string {
	values, _ := url.ParseQuery(rawQuery)
	values.Del(ExpiresParam)
	values.Del(SignatureParam)
	return values.Encode()
}

func signingPayload(method, path, canonicalQuery, expires string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + "\n" + path + "\n" + canonicalQuery + "\n" + expires
}
