package resterr

import (
	"fmt"
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

// Error implements the error interface.
func (r RESTErr) Error() string {
	return fmt.Sprintf(
		"status code: '%d', message: '%s', json: '%s'",
		r.StatusCode, r.Message, string(r.json),
	)
}
