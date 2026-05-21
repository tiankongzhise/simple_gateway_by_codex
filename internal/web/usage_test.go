package web

import "testing"

func TestBuildUsageDocumentIncludesGatewayAuthHeaders(t *testing.T) {
	doc := BuildUsageDocument("https://gateway.example.com")

	if doc.ServiceName == "" {
		t.Fatal("missing service name")
	}
	if doc.Conventions["baseURL"] != "https://gateway.example.com" {
		t.Fatalf("baseURL = %v", doc.Conventions["baseURL"])
	}

	headers := map[string]bool{}
	for _, header := range doc.Gateway.Headers {
		headers[header.Name] = true
	}
	for _, required := range []string{"Access-Token", "Service-Name", "Origin", "Referer", "model"} {
		if !headers[required] {
			t.Fatalf("missing gateway header %s", required)
		}
	}
}

func TestBuildUsageDocumentDoesNotExposePrivateRouteData(t *testing.T) {
	doc := BuildUsageDocument("")

	if doc.Gateway.PrivateData == "" {
		t.Fatal("expected privacy statement")
	}
	if len(doc.Endpoints) == 0 {
		t.Fatal("expected endpoints")
	}
}
