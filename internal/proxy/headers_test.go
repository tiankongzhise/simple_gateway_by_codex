package proxy

import (
	"net/http"
	"testing"

	"simple_gateway_by_codex/internal/models"
)

func TestCopyProxyRequestHeaderSkipsHopByHop(t *testing.T) {
	src := http.Header{}
	src.Set("Connection", "close")
	src.Set("X-Trace", "abc")
	dst := http.Header{}

	copyProxyRequestHeader(dst, src)

	if dst.Get("Connection") != "" {
		t.Fatal("Connection header should not be copied")
	}
	if dst.Get("X-Trace") != "abc" {
		t.Fatalf("X-Trace = %q", dst.Get("X-Trace"))
	}
}

func TestApplyHeaderRules(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Remove", "yes")

	applyHeaderRules(headers, []models.HeaderRule{
		{Operation: "set", HeaderName: "X-New", HeaderValue: "value"},
		{Operation: "remove", HeaderName: "X-Remove"},
	})

	if headers.Get("X-New") != "value" {
		t.Fatalf("X-New = %q", headers.Get("X-New"))
	}
	if headers.Get("X-Remove") != "" {
		t.Fatal("X-Remove should be removed")
	}
}
