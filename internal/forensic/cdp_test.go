package forensic

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func testPort(t *testing.T, server *httptest.Server) int {
	t.Helper()
	parts := strings.Split(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])
	return port
}

func TestCaptureTabs_Valid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"title": "YouTube", "type": "page", "url": "https://youtube.com", "id": "abc", "webSocketDebuggerUrl": "ws://..."},
			{"title": "Background", "type": "service_worker", "url": "chrome-extension://xyz"}
		]`))
	}))
	defer server.Close()

	tabs, err := CaptureTabs(testPort(t, server))
	if err != nil {
		t.Fatalf("CaptureTabs: %v", err)
	}
	if len(tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(tabs))
	}
	if tabs[0].Title != "YouTube" || tabs[0].Type != "page" || tabs[0].URL != "https://youtube.com" {
		t.Errorf("tab 0 = %+v", tabs[0])
	}
	if tabs[1].Type != "service_worker" {
		t.Errorf("tab 1 type = %q, want service_worker", tabs[1].Type)
	}
}

func TestCaptureTabs_EmptyArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	tabs, err := CaptureTabs(testPort(t, server))
	if err != nil {
		t.Fatalf("CaptureTabs: %v", err)
	}
	if tabs == nil {
		t.Error("empty response should return empty slice, not nil")
	}
	if len(tabs) != 0 {
		t.Errorf("got %d tabs, want 0", len(tabs))
	}
}

func TestCaptureTabs_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tabs, err := CaptureTabs(testPort(t, server))
	if err == nil {
		t.Error("expected error for non-200 status")
	}
	if tabs != nil {
		t.Error("expected nil tabs on error")
	}
}

func TestCaptureTabs_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	tabs, err := CaptureTabs(testPort(t, server))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
	if tabs != nil {
		t.Error("expected nil tabs on error")
	}
}

func TestCaptureTabs_InvalidPort(t *testing.T) {
	_, err := CaptureTabs(0)
	if err == nil {
		t.Error("expected error for port 0")
	}
	_, err = CaptureTabs(70000)
	if err == nil {
		t.Error("expected error for port 70000")
	}
}

func TestCaptureTabs_UnknownFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"title": "Test", "type": "page", "url": "http://test.com", "newField": "ignored"}]`))
	}))
	defer server.Close()

	tabs, err := CaptureTabs(testPort(t, server))
	if err != nil {
		t.Fatalf("unexpected error with unknown fields: %v", err)
	}
	if len(tabs) != 1 || tabs[0].Title != "Test" {
		t.Errorf("unexpected result: %+v", tabs)
	}
}
