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

	modes, ok := doc.Conventions["accessModes"].([]string)
	if !ok || len(modes) == 0 {
		t.Fatal("missing access mode conventions")
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

func TestBuildUsageDocumentIncludesSignedLinkEndpoint(t *testing.T) {
	doc := BuildUsageDocument("")

	for _, endpoint := range doc.Endpoints {
		if endpoint.Method == "POST" && endpoint.Path == "/api/routes/{id}/signed-link" {
			return
		}
	}
	t.Fatal("missing signed link endpoint")
}
