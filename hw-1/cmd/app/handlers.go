package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"ddia/hw1/internal/models"
	"ddia/hw1/internal/store"
)

type api struct {
	store *store.Store
}

type itemRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /items", a.listItems)
	mux.HandleFunc("POST /items", a.createItem)
	mux.HandleFunc("GET /items/{id}", a.getItem)
	mux.HandleFunc("PUT /items/{id}", a.updateItem)
}

func (a *api) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) listItems(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.List(r.Context())
	if err != nil {
		writeStoreError(w, err)

		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *api) createItem(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeItem(w, r)
	if !ok {
		return
	}
	it, err := a.store.Create(r.Context(), req.Name, req.Value)
	if err != nil {
		writeStoreError(w, err)

		return
	}
	writeJSON(w, http.StatusCreated, it)
}

func (a *api) getItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	it, err := a.store.Get(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)

		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (a *api) updateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	req, ok := decodeItem(w, r)
	if !ok {
		return
	}
	it, err := a.store.Update(r.Context(), id, req.Name, req.Value)
	if err != nil {
		writeStoreError(w, err)

		return
	}
	writeJSON(w, http.StatusOK, it)
}

func decodeItem(w http.ResponseWriter, r *http.Request) (itemRequest, bool) {
	var req itemRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())

		return req, false
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "field 'name' is required")

		return req, false
	}

	return req, true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")

		return 0, false
	}

	return id, true
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())

		return
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusServiceUnavailable, "request cancelled")

		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "err", err)
	}
}
