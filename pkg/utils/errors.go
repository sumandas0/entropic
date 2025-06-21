package utils

import (
	"errors"
	"fmt"
)

// Common error types
var (
	// ErrNotFound indicates a requested resource was not found
	ErrNotFound = errors.New("resource not found")
	
	// ErrAlreadyExists indicates a resource already exists
	ErrAlreadyExists = errors.New("resource already exists")
	
	// ErrInvalidInput indicates invalid input parameters
	ErrInvalidInput = errors.New("invalid input")
	
	// ErrUnauthorized indicates unauthorized access
	ErrUnauthorized = errors.New("unauthorized")
	
	// ErrForbidden indicates forbidden access
	ErrForbidden = errors.New("forbidden")
	
	// ErrInternal indicates an internal error
	ErrInternal = errors.New("internal error")
	
	// ErrTimeout indicates a timeout occurred
	ErrTimeout = errors.New("operation timeout")
	
	// ErrConcurrentModification indicates concurrent modification conflict
	ErrConcurrentModification = errors.New("concurrent modification detected")
	
	// ErrValidation indicates validation failure
	ErrValidation = errors.New("validation failed")
	
	// ErrTransactionFailed indicates transaction failure
	ErrTransactionFailed = errors.New("transaction failed")
)

// Error codes for API responses
const (
	CodeNotFound              = "NOT_FOUND"
	CodeAlreadyExists         = "ALREADY_EXISTS"
	CodeInvalidInput          = "INVALID_INPUT"
	CodeUnauthorized          = "UNAUTHORIZED"
	CodeForbidden             = "FORBIDDEN"
	CodeInternal              = "INTERNAL_ERROR"
	CodeTimeout               = "TIMEOUT"
	CodeConcurrentModification = "CONCURRENT_MODIFICATION"
	CodeValidation            = "VALIDATION_ERROR"
	CodeTransactionFailed     = "TRANSACTION_FAILED"
)

// AppError represents an application error with code and context
type AppError struct {
	Code    string
	Message string
	Err     error
	Details map[string]interface{}
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError creates a new application error
func NewAppError(code, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
		Details: make(map[string]interface{}),
	}
}

// WithDetails adds details to the error
func (e *AppError) WithDetails(details map[string]interface{}) *AppError {
	e.Details = details
	return e
}

// WithDetail adds a single detail to the error
func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// IsNotFound checks if error is a not found error
func IsNotFound(err error) bool {
	var appErr *AppError
	return errors.Is(err, ErrNotFound) || 
		(err != nil && errors.As(err, &appErr) && appErr.Code == CodeNotFound)
}

// IsAlreadyExists checks if error is an already exists error
func IsAlreadyExists(err error) bool {
	var appErr *AppError
	return errors.Is(err, ErrAlreadyExists) || 
		(err != nil && errors.As(err, &appErr) && appErr.Code == CodeAlreadyExists)
}

// IsValidation checks if error is a validation error
func IsValidation(err error) bool {
	var appErr *AppError
	return errors.Is(err, ErrValidation) || 
		(err != nil && errors.As(err, &appErr) && appErr.Code == CodeValidation)
}

// WrapError wraps an error with context
func WrapError(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}