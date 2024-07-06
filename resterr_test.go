package resterr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var logger = slog.New(slog.NewJSONHandler(io.Discard, nil))

func TestNewHandler(t *testing.T) {
	t.Parallel()

	errFoo := errors.New("foo err")
	errBar := errors.New("bar err")

	givenErrorMap := map[error]RESTErr{
		errFoo: {
			StatusCode: http.StatusTeapot,
			Message:    errFoo.Error(),
		},
		errBar: {
			StatusCode: http.StatusTooEarly,
			Message:    errBar.Error(),
		},
	}

	t.Run("without options", func(t *testing.T) {
		t.Parallel()

		observed, err := NewHandler(logger, givenErrorMap)
		require.NoError(t, err)

		assert.NotEmpty(t, observed.logger)

		var internalErrJSON RESTErr
		require.NoError(t, json.Unmarshal(observed.internalErrJSON, &internalErrJSON))

		assert.Equal(t, http.StatusInternalServerError, internalErrJSON.StatusCode)
		assert.Equal(t, "something went wrong", internalErrJSON.Message)

		assert.NotEmpty(t, observed.errorMap)
		assert.Len(t, observed.errorMap, len(givenErrorMap))

		_, foundFoo := observed.errorMap[errFoo]
		assert.True(t, foundFoo)

		_, foundBar := observed.errorMap[errBar]
		assert.True(t, foundBar)

		assert.Empty(t, observed.validationFn)
	})

	t.Run("with validation option", func(t *testing.T) {
		t.Parallel()

		passValidationFn := func(re RESTErr) error {
			return nil
		}

		failValidationFn := func(re RESTErr) error {
			return assert.AnError
		}

		testCases := []struct {
			name              string
			givenValidationFn func(h *Handler)
			expectedErr       error
		}{
			{
				name:              "returns no error",
				givenValidationFn: WithValidationFn(passValidationFn),
				expectedErr:       nil,
			},
			{
				name:              "returns error",
				givenValidationFn: WithValidationFn(failValidationFn),
				expectedErr:       assert.AnError,
			},
		}

		for _, tc := range testCases {
			_, err := NewHandler(logger, givenErrorMap, tc.givenValidationFn)
			assert.ErrorIs(t, err, tc.expectedErr)
		}
	})
}

type mockLogWriter struct {
	writeFunc       func(p []byte) (n int, err error)
	writeHeaderFunc func(statusCode int)
	headerFunc      func() http.Header
}

func (m *mockLogWriter) Write(p []byte) (n int, err error) {
	return m.writeFunc(p)
}

func (m *mockLogWriter) WriteHeader(statusCode int) {
	m.writeHeaderFunc(statusCode)
}

func (m *mockLogWriter) Header() http.Header {
	return m.headerFunc()
}

func TestHandle(t *testing.T) {
	t.Parallel()

	errFoo := errors.New("foo err")
	errBar := errors.New("bar err")

	errorMap := map[error]RESTErr{
		errFoo: {
			StatusCode: http.StatusTeapot,
			Message:    errFoo.Error(),
		},
		errBar: {
			StatusCode: http.StatusTooEarly,
			Message:    errBar.Error(),
		},
	}

	testCases := []struct {
		name               string
		givenErr           error
		expectedStatusCode int
		expectedLogLvl     string
		expectedErr        RESTErr
	}{
		{
			name:               "mapped error",
			givenErr:           errFoo,
			expectedStatusCode: http.StatusTeapot,
			expectedLogLvl:     "INFO",
			expectedErr:        errorMap[errFoo],
		},
		{
			name:               "unmapped error",
			givenErr:           errors.New("qux error"),
			expectedStatusCode: http.StatusInternalServerError,
			expectedLogLvl:     "ERROR",
			expectedErr:        internalErr,
		},
		{
			name: "RESTErr sent directly to handler",
			givenErr: RESTErr{
				StatusCode: http.StatusEarlyHints,
				Message:    "message",
			},
			expectedStatusCode: http.StatusEarlyHints,
			expectedLogLvl:     "INFO",
			expectedErr: RESTErr{
				StatusCode: http.StatusEarlyHints,
				Message:    "message",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var logData string

			logWriter := mockLogWriter{
				writeFunc: func(p []byte) (n int, err error) {
					logData = string(p)
					return 0, nil
				},
			}

			logger := slog.New(slog.NewTextHandler(&logWriter, nil))

			handler, err := NewHandler(logger, errorMap)
			require.NoError(t, err)

			writer := httptest.NewRecorder()

			handler.Handle(context.TODO(), writer, tc.givenErr)

			assert.Equal(t, tc.expectedStatusCode, writer.Result().StatusCode)
			assert.Equal(t, "application/json", writer.Result().Header["Content-Type"][0])

			var result RESTErr
			require.NoError(t, json.NewDecoder(writer.Result().Body).Decode(&result))

			assert.Equal(t, tc.expectedErr, result)

			assert.True(t, strings.Contains(logData, tc.givenErr.Error()))
			assert.True(t, strings.Contains(logData, tc.expectedLogLvl))
		})
	}
}

func TestWriteInternalErr(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(slog.Default(), map[error]RESTErr{})
	require.NoError(t, err)

	payload := httptest.NewRecorder()

	handler.writeInternalErr(context.TODO(), payload)

	assert.Equal(t, http.StatusInternalServerError, payload.Result().StatusCode)

	var result RESTErr
	require.NoError(t, json.NewDecoder(payload.Result().Body).Decode(&result))

	assert.Equal(t, internalErr, result)
}

func TestWrite(t *testing.T) {
	t.Parallel()

	errWithJSON := RESTErr{
		StatusCode: 123,
		Message:    "foo-message",
	}

	b, err := json.Marshal(errWithJSON)
	require.NoError(t, err)

	errWithJSON.json = b

	errWithoutJSON := RESTErr{
		StatusCode: 456,
		Message:    "bar-message",
	}

	bb, err := json.Marshal(errWithoutJSON)
	require.NoError(t, err)

	testCases := []struct {
		name         string
		givenErr     RESTErr
		expectedJSON []byte
	}{
		{
			name:         "error contain pre-compiled JSON",
			givenErr:     errWithJSON,
			expectedJSON: b,
		},
		{
			name:         "error does not contain pre-compiled JSON",
			givenErr:     errWithoutJSON,
			expectedJSON: bb,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				writeHeaderCalled bool
				writeCalled       bool
			)

			w := mockLogWriter{
				writeHeaderFunc: func(statusCode int) {
					writeHeaderCalled = true
					assert.Equal(t, tc.givenErr.StatusCode, statusCode)
				},
				writeFunc: func(p []byte) (n int, err error) {
					writeCalled = true
					assert.Equal(t, tc.expectedJSON, p)
					return 0, nil
				},
			}

			h := Handler{}

			h.write(context.TODO(), &w, tc.givenErr)

			require.True(t, writeHeaderCalled)
			require.True(t, writeCalled)
		})
	}
}
