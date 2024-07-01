package resterr

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHandler(t *testing.T) {
	t.Parallel()

	givenLogger := slog.Default()

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

		observed, err := NewHandler(givenLogger, givenErrorMap)
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
			_, err := NewHandler(givenLogger, givenErrorMap, tc.givenValidationFn)
			assert.ErrorIs(t, err, tc.expectedErr)
		}
	})
}

func TestHandle(t *testing.T) {
	t.Parallel()

	logger := slog.Default()

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
	}{
		{
			name:               "mapped error",
			givenErr:           errFoo,
			expectedStatusCode: http.StatusTeapot,
		},
		{
			name:               "unmapped error",
			givenErr:           errors.New("qux error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, err := NewHandler(logger, errorMap)
			require.NoError(t, err)

			payload := httptest.NewRecorder()

			handler.Handle(context.TODO(), payload, tc.givenErr)

			// TODO(alesr): assert log messages, level and context

			assert.Equal(t, tc.expectedStatusCode, payload.Result().StatusCode)
			assert.Equal(t, "application/json", payload.Result().Header["Content-Type"][0])

			var result RESTErr
			require.NoError(t, json.NewDecoder(payload.Result().Body).Decode(&result))

			expectedErr, mapped := errorMap[tc.givenErr]
			if !mapped {
				assert.Equal(t, internalErr, result)
			} else {
				assert.Equal(t, expectedErr, result)
			}
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
