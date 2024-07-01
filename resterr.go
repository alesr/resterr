package resterr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

var internalErr = RESTErr{
	StatusCode: http.StatusInternalServerError,
	Message:    "something went wrong",
}

// RESTErr represents a RESTful error.
// The json field is used to pre-marshal the error into JSON format.
type RESTErr struct {
	StatusCode int    `json:"status-code"`
	Message    string `json:"message"`
	json       []byte `json:"-"`
}

// Handler handles standard errors by logging them and looking for an equivalent REST error in the error map.
// Errors that are not mapped result in internal server errors.
type Handler struct {
	logger          *slog.Logger
	internalErrJSON []byte
	errorMap        map[error]*RESTErr
	validationFn    func(restErr RESTErr) error
}

// Option applies custom behavior to the handler.
type Option func(h *Handler)

// WithValidationFn is an option to set a custom validation function for REST errors.
func WithValidationFn(fn func(restErr RESTErr) error) Option {
	return func(h *Handler) {
		h.validationFn = fn
	}
}

// NewHandler returns a REST error handler.
// It pre-processes the REST errors' JSON values.
func NewHandler(logger *slog.Logger, errorMap map[error]RESTErr, opts ...Option) (*Handler, error) {
	internalErrJSON, err := json.Marshal(internalErr)
	if err != nil {
		return nil, fmt.Errorf("could not marshal internal err: %w", err)
	}

	h := Handler{
		logger:          logger.WithGroup("resterr-handler"),
		errorMap:        make(map[error]*RESTErr, len(errorMap)),
		internalErrJSON: internalErrJSON,
	}

	for _, o := range opts {
		o(&h)
	}

	for k, e := range errorMap {
		if h.validationFn != nil {
			if err := h.validationFn(e); err != nil {
				return nil, fmt.Errorf("validation failed for REST error '%v': %w", e, err)
			}
		}

		res, err := json.Marshal(&e)
		if err != nil {
			return nil, fmt.Errorf("could not marshal REST error '%v': %w", e, err)
		}
		e.json = res

		h.errorMap[k] = &e
	}
	return &h, nil
}

// Handle logs the original error and checks for the error in the error -> REST error map
// provided at initialization. If the error is present in the map, it writes the REST error as JSON.
// Otherwise, it writes a JSON indicating an internal server error.
func (h *Handler) Handle(ctx context.Context, w http.ResponseWriter, err error) {
	h.logger.Info("Handling error.", slog.String("error", err.Error()))

	w.Header().Set("Content-Type", "application/json")

	for k, v := range h.errorMap {
		if errors.Is(err, k) {
			w.WriteHeader(v.StatusCode)
			if _, err := w.Write(v.json); err != nil {
				h.logger.ErrorContext(ctx, "Failed to write JSON error.", slog.String("error", err.Error()))
				h.writeInternalErr(ctx, w)
			}
			return
		}
	}
	h.writeInternalErr(ctx, w)
}

// writeInternalErr writes an internal server error JSON response and logs any errors that occur while writing.
func (h *Handler) writeInternalErr(ctx context.Context, w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	if _, err := w.Write(h.internalErrJSON); err != nil {
		h.logger.ErrorContext(ctx, "Failed to write internal JSON error.", slog.String("error", err.Error()))
	}
}
