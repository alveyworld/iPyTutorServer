package main

import (
	"errors"
	"io"
	"net"
	"testing"
)

func TestIsRetryableOpenRouterError(t *testing.T) {
	if !isRetryableOpenRouterError(io.ErrUnexpectedEOF) {
		t.Fatal("expected unexpected EOF to be retryable")
	}
	if isRetryableOpenRouterError(errors.New("bad request")) {
		t.Fatal("expected generic error to not be retryable")
	}

	var timeout net.Error = fakeNetError{timeout: true}
	if !isRetryableOpenRouterError(timeout) {
		t.Fatal("expected timeout to be retryable")
	}
}

type fakeNetError struct {
	timeout bool
}

func (e fakeNetError) Error() string   { return "timeout" }
func (e fakeNetError) Timeout() bool   { return e.timeout }
func (e fakeNetError) Temporary() bool { return true }
