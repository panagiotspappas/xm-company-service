package httpapi

import (
	"context"
	"net/http"
	"time"
)

const readinessTimeout = time.Second

// ReadinessChecker defines the dependency check required by the readiness endpoint.
type ReadinessChecker interface {
	Ping(context.Context) error
}

type healthHandler struct {
	readiness ReadinessChecker
	timeout   time.Duration
}

type healthResponse struct {
	Status string `json:"status"`
}

func (handler healthHandler) live(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, healthResponse{Status: "ok"})
}

func (handler healthHandler) ready(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), handler.timeout)
	defer cancel()

	if err := handler.readiness.Ping(ctx); err != nil || ctx.Err() != nil {
		if request.Context().Err() == context.Canceled {
			return
		}

		writeError(
			writer,
			http.StatusServiceUnavailable,
			errorCodeServiceUnavailable,
			"service unavailable",
		)
		return
	}

	writeJSON(writer, http.StatusOK, healthResponse{Status: "ok"})
}
