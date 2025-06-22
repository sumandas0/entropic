package utils

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound               = errors.New("resource not found")
	ErrAlreadyExists          = errors.New("resource already exists")
	ErrInvalidInput           = errors.New("invalid input")
	ErrUnauthorized           = errors.New("unauthorized")
	ErrForbidden              = errors.New("forbidden")
	ErrInternal               = errors.New("internal error")
	ErrTimeout                = errors.New("operation timeout")
	ErrConcurrentModification = errors.New("concurrent modification detected")
	ErrValidation             = errors.New("validation failed")
	ErrTransactionFailed      = errors.New("transaction failed")
)

const (
	CodeNotFound               = "NOT_FOUND"
	CodeAlreadyExists          = "ALREADY_EXISTS"
	CodeInvalidInput           = "INVALID_INPUT"
	CodeUnauthorized           = "UNAUTHORIZED"
	CodeForbidden              = "FORBIDDEN"
	CodeInternal               = "INTERNAL_ERROR"
	CodeTimeout                = "TIMEOUT"
	CodeConcurrentModification = "CONCURRENT_MODIFICATION"
	CodeValidation             = "VALIDATION_ERROR"
	CodeTransactionFailed      = "TRANSACTION_FAILED"
)

type AppError struct {
	Code    string
	Message string
	Err     error
	Details map[string]interface{}
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewAppError(code, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
		Details: make(map[string]interface{}),
	}
}

func (e *AppError) WithDetails(details map[string]interface{}) *AppError {
	e.Details = details
	return e
}

func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

func IsNotFound(err error) bool {
	var appErr *AppError
	return errors.Is(err, ErrNotFound) ||
		(err != nil && errors.As(err, &appErr) && appErr.Code == CodeNotFound)
}

func IsAlreadyExists(err error) bool {
	var appErr *AppError
	return errors.Is(err, ErrAlreadyExists) ||
		(err != nil && errors.As(err, &appErr) && appErr.Code == CodeAlreadyExists)
}

func IsValidation(err error) bool {
	var appErr *AppError
	return errors.Is(err, ErrValidation) ||
		(err != nil && errors.As(err, &appErr) && appErr.Code == CodeValidation)
}

func WrapError(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}
