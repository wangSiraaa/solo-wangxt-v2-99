package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/example/ab-platform/internal/assignment"
	"github.com/example/ab-platform/internal/storage"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRejectedUserHasNoExposure(t *testing.T) {
	router := NewServer(storage.NewMemoryStore(storage.SeedConfig())).Router()

	body := postJSON(t, router, "/api/decide", map[string]any{
		"subject": map[string]any{
			"user_id":    "unknown-user-x",
			"registered": false,
			"country":    "US",
			"plan":       "paid",
			"source":     "synthetic",
		},
		"record": true,
		"expose": true,
	})
	if body["exposed"] != false {
		t.Fatalf("rejected user must not be exposed: %v", body)
	}
	decision := body["decision"].(map[string]any)
	if decision["reject_reason"] != assignment.ReasonUnknownIdentity {
		t.Fatalf("unexpected decision: %v", decision)
	}

	statsBody := getJSON(t, router, "/api/stats?source=synthetic")
	if statsBody["exposure_total"] != float64(0) {
		t.Fatalf("expected no exposures, got %v", statsBody["exposure_total"])
	}
	if statsBody["assignment_total"] != float64(1) {
		t.Fatalf("expected diagnostic assignment, got %v", statsBody)
	}
}

func TestWhitelistExposureIsSeparatelyLabeled(t *testing.T) {
	router := NewServer(storage.NewMemoryStore(storage.SeedConfig())).Router()

	body := postJSON(t, router, "/api/decide", map[string]any{
		"subject": map[string]any{
			"user_id":    "vip-001",
			"registered": true,
			"country":    "CA",
			"plan":       "free",
			"source":     "synthetic",
		},
		"record": true,
		"expose": true,
	})
	if body["exposed"] != true {
		t.Fatalf("assigned whitelist should expose: %v", body)
	}
	statsBody := getJSON(t, router, "/api/stats?source=synthetic")
	exposures := statsBody["exposures"].(map[string]any)
	if exposures["whitelist:rewards_panel/treatment"] != float64(1) {
		t.Fatalf("whitelist exposure was not separated: %v", exposures)
	}
	if _, normal := exposures["rewards_panel/treatment"]; normal {
		t.Fatalf("whitelist exposure leaked into normal metric: %v", exposures)
	}
}

func TestRepeatedAssignedDecisionKeepsSingleExposure(t *testing.T) {
	router := NewServer(storage.NewMemoryStore(storage.SeedConfig())).Router()
	first := postJSON(t, router, "/api/decide", assignedPayload("syn-3340"))
	second := postJSON(t, router, "/api/decide", assignedPayload("syn-3340"))
	if first["exposure_id"] != second["exposure_id"] {
		t.Fatalf("expected same stable exposure id, got %v and %v", first["exposure_id"], second["exposure_id"])
	}
	statsBody := getJSON(t, router, "/api/stats?source=synthetic")
	if statsBody["exposure_total"] != float64(1) {
		t.Fatalf("repeated decision duplicated exposure: %v", statsBody)
	}
}

func assignedPayload(userID string) map[string]any {
	return map[string]any{
		"subject": map[string]any{
			"user_id":    userID,
			"registered": true,
			"country":    "US",
			"plan":       "free",
			"source":     "synthetic",
		},
		"record": true,
		"expose": true,
	}
}

func postJSON(t *testing.T, router http.Handler, path string, payload map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s status %d: %s", path, rec.Code, rec.Body.String())
	}
	return decode(t, rec)
}

func getJSON(t *testing.T, router http.Handler, path string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status %d: %s", path, rec.Code, rec.Body.String())
	}
	return decode(t, rec)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
