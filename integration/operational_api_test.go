//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestReadinessReportsPostgresAvailable(t *testing.T) {
	api := newIntegrationAPI(t)

	response, err := api.client.Get(api.baseURL + "/health/ready")
	if err != nil {
		t.Fatalf("GET readiness: %v", err)
	}
	defer closeResponseBody(t, response)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("readiness status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("readiness status body = %q, want ok", body.Status)
	}
}
