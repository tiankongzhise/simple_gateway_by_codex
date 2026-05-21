package linksign

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSignerVerifiesAndStripsSignatureParams(t *testing.T) {
	signer := New("secret")
	expiresAt := time.Unix(2000, 0)
	rawQuery := signer.AddSignature("get", "/gw/alice/report", "b=2&a=1", expiresAt)

	businessQuery, err := signer.Verify("GET", "/gw/alice/report", rawQuery, time.Unix(1999, 0))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if businessQuery != "a=1&b=2" {
		t.Fatalf("businessQuery = %q", businessQuery)
	}
	if strings.Contains(businessQuery, ExpiresParam) || strings.Contains(businessQuery, SignatureParam) {
		t.Fatalf("business query leaked signature params: %q", businessQuery)
	}
}

func TestSignerRejectsTamperedPathMethodQueryAndExpiredLink(t *testing.T) {
	signer := New("secret")
	expiresAt := time.Unix(2000, 0)
	rawQuery := signer.AddSignature("GET", "/gw/alice/report", "a=1", expiresAt)

	tests := []struct {
		name   string
		method string
		path   string
		query  string
		now    time.Time
		err    error
	}{
		{name: "method", method: "POST", path: "/gw/alice/report", query: rawQuery, now: time.Unix(1999, 0), err: ErrInvalidSignature},
		{name: "path", method: "GET", path: "/gw/alice/other", query: rawQuery, now: time.Unix(1999, 0), err: ErrInvalidSignature},
		{name: "query", method: "GET", path: "/gw/alice/report", query: rawQuery + "&a=2", now: time.Unix(1999, 0), err: ErrInvalidSignature},
		{name: "expired", method: "GET", path: "/gw/alice/report", query: rawQuery, now: time.Unix(2000, 0), err: ErrExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := signer.Verify(tt.method, tt.path, tt.query, tt.now)
			if !errors.Is(err, tt.err) {
				t.Fatalf("Verify() error = %v, want %v", err, tt.err)
			}
		})
	}
}
