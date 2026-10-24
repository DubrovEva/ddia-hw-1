package grafana

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertDashboard(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/dashboards/db" {
			http.NotFound(w, r)

			return
		}
		if r.Header.Get("Authorization") != "Bearer glsa_test" {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)

			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		writeTestJSON(w, DashboardResult{UID: "items", URL: "/d/items/x", Status: "success", Version: 3})
	}))
	t.Cleanup(srv.Close)

	res, err := NewTokenAuth(srv.URL, "glsa_test").UpsertDashboard(t.Context(), map[string]any{"uid": "items"}, "generated")
	require.NoError(t, err)
	assert.Equal(t, DashboardResult{UID: "items", URL: "/d/items/x", Status: "success", Version: 3}, res)
	assert.Equal(t, map[string]any{
		"dashboard": map[string]any{"uid": "items"},
		"overwrite": true,
		"message":   "generated",
	}, got)

	_, err = NewTokenAuth(srv.URL, "wrong").UpsertDashboard(t.Context(), map[string]any{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}
