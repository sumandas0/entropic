package sdk

import (
	"fmt"
)

// ErrorType represents the type of error returned by the API
type ErrorType string

const (
	ErrorTypeValidation    ErrorType = "validation"
	ErrorTypeNotFound      ErrorType = "not_found"
	ErrorTypeAlreadyExists ErrorType = "already_exists"
	ErrorTypeInternal      ErrorType = "internal"
	ErrorTypeUnknown       ErrorType = "unknown"
)

// APIError represents an error response from the Entropic API
type APIError struct {
	Type    ErrorType              `json:"type"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
	Code    int                    `json:"-"` // HTTP status code
}

func (e *APIError) Error() string {
	if e.Details != nil {
		return fmt.Sprintf("%s: %s (details: %v)", e.Type, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// IsValidation returns true if the error is a validation error
func (e *APIError) IsValidation() bool {
	return e.Type == ErrorTypeValidation
}

// IsNotFound returns true if the error is a not found error
func (e *APIError) IsNotFound() bool {
	return e.Type == ErrorTypeNotFound
}

// IsAlreadyExists returns true if the error is an already exists error
func (e *APIError) IsAlreadyExists() bool {
	return e.Type == ErrorTypeAlreadyExists
}

// IsInternal returns true if the error is an internal server error
func (e *APIError) IsInternal() bool {
	return e.Type == ErrorTypeInternal
}

// IsAPIError checks if an error is an APIError
func IsAPIError(err error) bool {
	_, ok := err.(*APIError)
	return ok
}

// AsAPIError attempts to cast an error to APIError
func AsAPIError(err error) (*APIError, bool) {
	apiErr, ok := err.(*APIError)
	return apiErr, ok
}