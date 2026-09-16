package acme

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *CloudflareClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &CloudflareClient{Token: "test-token", HTTPClient: srv.Client(), BaseURL: srv.URL}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encoding test response: %v", err)
	}
}

func TestVerifyTokenSuccess(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q", got)
		}
		writeJSON(t, w, map[string]any{"success": true})
	})
	if err := client.VerifyToken(context.Background()); err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
}

func TestVerifyTokenFailure(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"success": false,
			"errors":  []map[string]any{{"code": 1000, "message": "Invalid API Token"}},
		})
	})
	if err := client.VerifyToken(context.Background()); err == nil {
		t.Fatal("expected an error for an invalid token")
	}
}

func TestFindZoneIDFallsBackToParentZone(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "example.com" {
			writeJSON(t, w, map[string]any{"success": true, "result": []map[string]any{{"id": "zone123"}}})
			return
		}
		writeJSON(t, w, map[string]any{"success": true, "result": []map[string]any{}})
	})
	zoneID, err := client.FindZoneID(context.Background(), "sub.example.com")
	if err != nil {
		t.Fatalf("FindZoneID: %v", err)
	}
	if zoneID != "zone123" {
		t.Errorf("zoneID = %q, want zone123", zoneID)
	}
}

func TestFindZoneIDNotFound(t *testing.T) {
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"success": true, "result": []map[string]any{}})
	})
	if _, err := client.FindZoneID(context.Background(), "example.com"); err == nil {
		t.Fatal("expected an error when no zone is found")
	}
}

func TestCreateAndListAndDeleteTXT(t *testing.T) {
	created := false
	deleted := false
	client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/zones/zone123/dns_records":
			created = true
			writeJSON(t, w, map[string]any{"success": true, "result": map[string]any{"id": "rec1", "content": "abc"}})
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone123/dns_records":
			writeJSON(t, w, map[string]any{"success": true, "result": []map[string]any{{"id": "rec1", "content": "abc"}}})
		case r.Method == http.MethodDelete && r.URL.Path == "/zones/zone123/dns_records/rec1":
			deleted = true
			writeJSON(t, w, map[string]any{"success": true, "result": map[string]any{"id": "rec1"}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	recordID, err := client.CreateTXT(context.Background(), "zone123", "_acme-challenge.example.com", "abc")
	if err != nil {
		t.Fatalf("CreateTXT: %v", err)
	}
	if recordID != "rec1" {
		t.Errorf("recordID = %q, want rec1", recordID)
	}
	if !created {
		t.Error("expected a POST to create the record")
	}

	recs, err := client.ListTXT(context.Background(), "zone123", "_acme-challenge.example.com")
	if err != nil {
		t.Fatalf("ListTXT: %v", err)
	}
	if len(recs) != 1 || recs[0].Content != "abc" {
		t.Errorf("ListTXT = %+v, want one record with content abc", recs)
	}

	if err := client.DeleteTXT(context.Background(), "zone123", recs[0].ID); err != nil {
		t.Fatalf("DeleteTXT: %v", err)
	}
	if !deleted {
		t.Error("expected a DELETE for the record")
	}
}
