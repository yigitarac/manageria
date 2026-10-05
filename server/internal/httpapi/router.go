package httpapi

import (
	"log/slog"
	"net/http"
)

// NewRouter registers the generated OpenAPI routes on a fresh mux and returns it.
func NewRouter(logger *slog.Logger) http.Handler {
	strict := NewStrictHandler(NewServer(logger), nil)
	return HandlerFromMux(strict, http.NewServeMux())
}
