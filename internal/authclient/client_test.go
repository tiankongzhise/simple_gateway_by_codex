package authclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestServiceGroupToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/service-groups/token/latest" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if req["serviceGroupName"] != "group" || req["authorizationCode"] != "code" {
			t.Fatalf("request = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(GroupToken{AccessToken: "token", AccessTokenExpiresAt: 1})
	}))
	defer server.Close()

	client := New(server.URL)
	got, err := client.LatestServiceGroupToken(t.Context(), "group", "code")
	if err != nil {
		t.Fatalf("LatestServiceGroupToken() error = %v", err)
	}
	if got.AccessToken != "token" {
		t.Fatalf("AccessToken = %q", got.AccessToken)
	}
}

func TestVerifySendsHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Service-Name") != "group" {
			t.Fatalf("Service-Name = %q", r.Header.Get("Service-Name"))
		}
		if r.Header.Get("Target-Service-Name") != "svc" {
			t.Fatalf("Target-Service-Name = %q", r.Header.Get("Target-Service-Name"))
		}
		if r.Header.Get("Access-Token") != "token" {
			t.Fatalf("Access-Token = %q", r.Header.Get("Access-Token"))
		}
		_ = json.NewEncoder(w).Encode(VerifyResult{OK: true, ServiceName: "svc", ServiceGroupName: "group"})
	}))
	defer server.Close()

	client := New(server.URL)
	got, err := client.Verify(t.Context(), VerifyHeaders{
		ServiceName:       "group",
		TargetServiceName: "svc",
		AccessToken:       "token",
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !got.OK {
		t.Fatal("expected ok")
	}
}

func TestAuthServiceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.LatestServiceGroupToken(t.Context(), "group", "bad")
	if err == nil {
		t.Fatal("expected error")
	}
	authErr, ok := err.(Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if authErr.StatusCode != http.StatusForbidden {
		t.Fatalf("StatusCode = %d", authErr.StatusCode)
	}
}
