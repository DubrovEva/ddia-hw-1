package golden

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

type HandlerInfo struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

type HandlersResponse struct {
	Deployment string        `json:"deployment"`
	Handlers   []HandlerInfo `json:"handlers"`
}

func (i *Instrumenter) Handlers() []HandlerInfo {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return slices.Clone(i.handlers)
}

func (i *Instrumenter) HandlersHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(HandlersResponse{Deployment: i.deployment, Handlers: i.Handlers()})
	})
}

func (i *Instrumenter) register(handlerID, pattern string) {
	info := parsePattern(pattern)
	info.ID = handlerID

	i.mu.Lock()
	defer i.mu.Unlock()

	for _, h := range i.handlers {
		if h.ID == handlerID {
			panic(fmt.Sprintf("golden: handler id %q is already registered for %s %s", handlerID, h.Method, h.Path))
		}
	}
	i.handlers = append(i.handlers, info)
}

func parsePattern(pattern string) HandlerInfo {
	var info HandlerInfo
	rest := strings.TrimSpace(pattern)
	if method, path, ok := strings.Cut(rest, " "); ok {
		info.Method = method
		rest = strings.TrimSpace(path)
	}
	if idx := strings.Index(rest, "/"); idx >= 0 {
		rest = rest[idx:]
	}
	info.Path = rest

	return info
}
