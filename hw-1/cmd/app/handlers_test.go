package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"ddia/hw1/internal/store"
)

func TestHandlers(t *testing.T) {
	mux := http.NewServeMux()
	(&api{store: store.New()}).routes(mux)

	cases := []struct {
		name, method, path, body string
		wantCode                 int
	}{
		{"healthz", "GET", "/healthz", "", http.StatusOK},
		{"create", "POST", "/items", `{"name":"a","value":"1"}`, http.StatusCreated},
		{"create bad json", "POST", "/items", `{bad json`, http.StatusBadRequest},
		{"create no name", "POST", "/items", `{"value":"no name"}`, http.StatusBadRequest},
		{"get", "GET", "/items/1", "", http.StatusOK},
		{"get missing", "GET", "/items/999", "", http.StatusNotFound},
		{"get bad id", "GET", "/items/abc", "", http.StatusBadRequest},
		{"update", "PUT", "/items/1", `{"name":"b","value":"2"}`, http.StatusOK},
		{"update missing", "PUT", "/items/999", `{"name":"b"}`, http.StatusNotFound},
		{"list", "GET", "/items", "", http.StatusOK},
		{"method not allowed", "DELETE", "/items/1", "", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)))
			assert.Equal(t, c.wantCode, rec.Code, rec.Body.String())
		})
	}
}
