package httpapi

import (
	"context"
	"log/slog"
)

// Server implements StrictServerInterface backed by the generated types.
type Server struct {
	logger *slog.Logger
}

// NewServer builds the API handler implementation.
func NewServer(logger *slog.Logger) StrictServerInterface {
	return &Server{logger: logger}
}

// GetHealthz reports service liveness. A future version also checks backing stores.
func (s *Server) GetHealthz(_ context.Context, _ GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200JSONResponse{Status: Ok}, nil
}
