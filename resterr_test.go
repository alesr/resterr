package resterr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRESTErr_Error(t *testing.T) {
	t.Parallel()

	expected := "status code: '123', message: 'abc', json: ''"

	observed := RESTErr{
		StatusCode: 123,
		Message:    "abc",
	}.Error()

	assert.Equal(t, expected, observed)
}
