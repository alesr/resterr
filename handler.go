package resterr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
)

// Handler handles standard errors by logging them and looking for an equivalent REST error in the error map.
// Errors that are not mapped result in internal server errors.
type Handler struct {
	logger          *slog.Logger
	internalErrJSON []byte
	errorMap        sync.Map
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

var logger = slog.New(slog.NewJSONHandler(io.Discard, nil))

// NewHandler returns a REST error handler.
// It pre-processes the JSON values for REST errors.
func NewHandler(logger *slog.Logger, errMap map[error]RESTErr, opts ...Option) (*Handler, error) {
	internalErrJSON, err := json.Marshal(internalErr)
	if err != nil {
		return nil, fmt.Errorf("could not marshal internal error: %w", err)
	}

	h := Handler{
		logger:          logger.WithGroup("resterr-handler"),
		errorMap:        sync.Map{},
		internalErrJSON: internalErrJSON,
	}

	for _, o := range opts {
		o(&h)
	}

	for k, e := range errMap {
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

		h.errorMap.Store(k, e)
	}
	return &h, nil
}

// Writer defines the interface for writing error data.
type Writer interface {
	Write([]byte) (int, error)
	WriteHeader(statusCode int)
	Header() http.Header
}

// Handle logs the original error and checks for the error in the error -> REST error map
// provided at initialization. If the error is present in the map, it writes the REST error as JSON.
// Otherwise, it writes a JSON indicating an internal server error.
func (h *Handler) Handle(ctx context.Context, w Writer, err error) {
	w.Header().Set("Content-Type", "application/json")

	var restErr RESTErr
	if errors.As(err, &restErr) {
		h.logger.InfoContext(ctx, "Handling REST error.", slog.String("error", err.Error()))
		h.write(ctx, w, restErr)
		return
	}

	var found bool
	h.errorMap.Range(func(k, v any) bool {
		keyErr, ok := k.(error)
		if !ok {
			h.logger.ErrorContext(ctx, "Failed to convert mapped key to error", slog.String("error", err.Error()))
			return false
		}

		if errors.Is(err, keyErr) {
			re, ok := v.(RESTErr)
			if !ok {
				h.logger.ErrorContext(ctx, "Failed to convert mapped value to RESTErr", slog.String("error", err.Error()))
				return false
			}

			found = true
			h.logger.InfoContext(ctx, "Handling mapped error.", slog.String("error", err.Error()), slog.String("rest-error", re.Error()))
			h.write(ctx, w, re)
			return true
		}
		return true
	})

	if found {
		return
	}

	h.logger.ErrorContext(ctx, "Handling unmapped error.", slog.String("error", err.Error()))
	h.writeInternalErr(ctx, w)
}

func (h *Handler) writeInternalErr(ctx context.Context, w Writer) {
	w.WriteHeader(http.StatusInternalServerError)
	if _, err := w.Write(h.internalErrJSON); err != nil {
		h.logger.ErrorContext(ctx, "Failed to write internal JSON error.", slog.String("error", err.Error()))
	}
}

func (h *Handler) write(ctx context.Context, w Writer, e RESTErr) {
	w.WriteHeader(e.StatusCode)

	// It's likely that we'll be handling mapped or unmapped errors.
	// They come with JSON bytes, as opposed to when RESTErr
	// errors are passed directly to the handler.
	payload := e.json

	var err error

	if e.json == nil {
		payload, err = json.Marshal(e)
		if err != nil {
			h.logger.ErrorContext(ctx, "Failed to marshal error during write", slog.String("source-error", e.Error()), slog.String("error", err.Error()))
			h.writeInternalErr(ctx, w)
			return
		}
	}

	if _, err := w.Write(payload); err != nil {
		h.logger.ErrorContext(ctx, "Failed to write JSON error.", slog.String("source-error", e.Error()), slog.String("error", err.Error()))
		h.writeInternalErr(ctx, w)
	}
}
