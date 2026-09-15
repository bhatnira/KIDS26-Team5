package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoCABRequestJSONSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":2}`))
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	got, err := doCABRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected parsed JSON object, got %T: %v", got, got)
	}
	if m["hits"] != float64(2) {
		t.Errorf("hits = %v, want 2", m["hits"])
	}
}

func TestDoCABRequestNon2xxReturnsStructuredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	got, err := doCABRequest(req)
	if err != nil {
		t.Fatalf("non-2xx must be a structured result, not a Go error: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected error map, got %T", got)
	}
	if m["status_code"] != http.StatusBadGateway {
		t.Errorf("status_code = %v, want %d", m["status_code"], http.StatusBadGateway)
	}
	if m["body"] != "upstream down" {
		t.Errorf("non-JSON body should pass through as string, got %v", m["body"])
	}
}
