# resterr

**resterr** is a Go package designed to together with proper propagation of errors through an application handle the logging and mapping of logic errors to RESTful errors. It helps log original errors, map them to predefined REST errors, and ensures unmapped errors result in a 500 Internal Server Error. This package is particularly useful for structuring application errors and standardizing API error responses.

## Features

- Centralized error handling for REST APIs.
- Logging of original errors while hiding internal details.
- Mapping of application errors to REST errors.
- Custom validation functions for REST errors.
- Standardized JSON responses for errors.

## Installation

To install `resterr`, use the following command:

```bash
go get github.com/alesr/resterr
```

## Usage

### Setting Up the Handler

First, initialize the logger and define your error mappings. Then, create the error handler using NewHandler.

```go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/alesr/resterr"
)

var (
	ErrNotFound = errors.New("not found")
	ErrBadRequest = errors.New("bad request")
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout))
	errorMap := map[error]resterr.RESTErr{
		ErrNotFound: {
			StatusCode: http.StatusNotFound,
			Message:    "The requested resource was not found",
		},
		ErrBadRequest: {
			StatusCode: http.StatusBadRequest,
			Message:    "The request was invalid",
		},
	}

	errHandler, err := resterr.NewHandler(logger, errorMap, resterr.WithValidationFn(validateRestErr))
	if err != nil {
		logger.Error("Failed to create handler", slog.String("error", err.Error()))
		return
	}

	http.HandleFunc("/example", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		errHandler.Handle(ctx, w, fmt.Errorf("could not do process example request: %w",ErrNotFound)) // Example usage
	})

	http.ListenAndServe(":8080", nil)
}

func validateRestErr(restErr resterr.RESTErr) error {
	if restErr.StatusCode < 400 || restErr.StatusCode >= 600 {
		return errors.New("invalid status code")
	}
	return nil
}
```

### Handling Errors

To handle errors in your HTTP handlers, use the Handle method of the Handler struct.

```go
func exampleHandler(w http.ResponseWriter, r *http.Request) {
    errHandler, _ := resterr.NewHandler(logger, errorMap)

	ctx := r.Context()
	err := someFunctionThatMayFail()
	if err != nil {
		errHandler.Handle(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Success"))
}
```

### Custom Validation Function

You can pass a custom validation function to the handler using the `WithValidationFn` option. This function will be used to validate each REST error during handler initialization.

```go
func validateRestErr(restErr resterr.RESTErr) error {
	if restErr.StatusCode < 400 || restErr.StatusCode >= 600 {
		return errors.New("invalid status code")
	}
	return nil
}
```

### Structs and Methods

#### RESTErr

Represents a RESTful error.

```go
type RESTErr struct {
	StatusCode int    `json:"status-code"`
	Message    string `json:"message"`
	json       []byte `json:"-"`
}
```

#### Handler

Handles errors by logging them and mapping them to REST errors.

```go
type Handler struct {
	logger          *slog.Logger
	internalErrJSON []byte
	errorMap        map[error]*RESTErr
	validationFn    func(restErr RESTErr) error
}
```

## Contributing
Contributions are welcome! Please open an issue or submit a pull request on GitHub.

## License
This project is licensed under the MIT License.
