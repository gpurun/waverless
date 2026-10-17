package gongji

import (
	"errors"
	"fmt"
)

// Platform error codes shared with the control contract.
const (
	CodeRegionUnavailable   = "REGION_UNAVAILABLE"
	CodeCapacityExhausted   = "CAPACITY_EXHAUSTED"
	CodeProviderRateLimited = "PROVIDER_RATE_LIMITED"
	CodeProviderTimeout     = "PROVIDER_TIMEOUT_UNKNOWN"
	CodeProviderRejected    = "PROVIDER_REJECTED"
)

// Error is a sanitized Gongji or catalog failure.
// Message never includes tokens, signing keys, or raw response bodies.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// AsError unwraps a Gongji error.
func AsError(err error) (*Error, bool) {
	var ge *Error
	if errors.As(err, &ge) {
		return ge, true
	}
	return nil, false
}

func vendorError(code, message string) *Error {
	if message == "" {
		message = "gongji request rejected"
	}
	return &Error{Code: code, Message: message}
}

func fmtError(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
