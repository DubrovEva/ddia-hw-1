package golden

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePattern(t *testing.T) {
	cases := map[string]HandlerInfo{
		"GET /items/{id}":          {Method: "GET", Path: "/items/{id}"},
		"POST /items":              {Method: "POST", Path: "/items"},
		"/any":                     {Path: "/any"},
		"GET example.com/items":    {Method: "GET", Path: "/items"},
		"  PUT   /items/{id}  ":    {Method: "PUT", Path: "/items/{id}"},
		"DELETE /items/{id...}":    {Method: "DELETE", Path: "/items/{id...}"},
		"GET /static/{$}":          {Method: "GET", Path: "/static/{$}"},
		"PATCH host.local/a/b/{c}": {Method: "PATCH", Path: "/a/b/{c}"},
	}
	for pattern, want := range cases {
		assert.Equal(t, want, parsePattern(pattern), pattern)
	}
}

func TestHandleRegistersHandlers(t *testing.T) {
	ins, _ := newTestInstrumenter(t)
	mux := http.NewServeMux()
	noop := func(http.ResponseWriter, *http.Request) {}

	ins.Handle(mux, "GET /items", "items_list", noop)
	ins.Handle(mux, "GET /items/{id}", "items_get", noop)

	assert.Equal(t, []HandlerInfo{
		{ID: "items_list", Method: "GET", Path: "/items"},
		{ID: "items_get", Method: "GET", Path: "/items/{id}"},
	}, ins.Handlers())
}

func TestWrapDoesNotRegister(t *testing.T) {
	ins, _ := newTestInstrumenter(t)
	ins.Wrap("h", http.NotFoundHandler())

	assert.Empty(t, ins.Handlers())
}

func TestHandleDuplicateIDPanics(t *testing.T) {
	ins, _ := newTestInstrumenter(t)
	mux := http.NewServeMux()
	noop := func(http.ResponseWriter, *http.Request) {}
	ins.Handle(mux, "GET /a", "same", noop)

	assert.Panics(t, func() { ins.Handle(mux, "GET /b", "same", noop) })
}

func TestHandlersHandler(t *testing.T) {
	ins, _ := newTestInstrumenter(t)
	mux := http.NewServeMux()
	ins.Handle(mux, "POST /items", "items_create", func(http.ResponseWriter, *http.Request) {})

	rec := serve(ins.HandlersHandler(), "GET", "/internal/handlers")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got HandlersResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, HandlersResponse{
		Deployment: "items",
		Handlers:   []HandlerInfo{{ID: "items_create", Method: "POST", Path: "/items"}},
	}, got)
}
