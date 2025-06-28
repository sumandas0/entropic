package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/sumandas0/entropic/pkg/utils"
)

// ErrorResponse represents the standard error response format
// @Description Standard error response format for all API errors
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains detailed error information
// @Description Detailed error information including code, message, and optional details
type ErrorDetail struct {
	Code      string         `json:"code" example:"NOT_FOUND"`
	Message   string         `json:"message" example:"Entity not found"`
	Details   map[string]any `json:"details,omitempty" swaggertype:"object"`
	Timestamp time.Time      `json:"timestamp" example:"2023-01-01T00:00:00Z"`
	RequestID string         `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
}

func ErrorHandler() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					handlePanic(w, r, err)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func SendError(w http.ResponseWriter, r *http.Request, err error, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	var errorResponse ErrorResponse

	if appErr, ok := err.(*utils.AppError); ok {
		errorResponse = ErrorResponse{
			Error: ErrorDetail{
				Code:      appErr.Code,
				Message:   appErr.Message,
				Details:   appErr.Details,
				Timestamp: time.Now().UTC(),
				RequestID: getRequestID(r),
			},
		}
	} else {

		errorResponse = ErrorResponse{
			Error: ErrorDetail{
				Code:      "INTERNAL_ERROR",
				Message:   err.Error(),
				Timestamp: time.Now().UTC(),
				RequestID: getRequestID(r),
			},
		}
	}

	json.NewEncoder(w).Encode(errorResponse)
}

func SendValidationError(w http.ResponseWriter, r *http.Request, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	errorResponse := ErrorResponse{
		Error: ErrorDetail{
			Code:      utils.CodeValidation,
			Message:   message,
			Details:   details,
			Timestamp: time.Now().UTC(),
			RequestID: getRequestID(r),
		},
	}

	json.NewEncoder(w).Encode(errorResponse)
}

func SendNotFoundError(w http.ResponseWriter, r *http.Request, resource string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)

	errorResponse := ErrorResponse{
		Error: ErrorDetail{
			Code:      utils.CodeNotFound,
			Message:   fmt.Sprintf("%s not found", resource),
			Timestamp: time.Now().UTC(),
			RequestID: getRequestID(r),
		},
	}

	json.NewEncoder(w).Encode(errorResponse)
}

func SendConflictError(w http.ResponseWriter, r *http.Request, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)

	errorResponse := ErrorResponse{
		Error: ErrorDetail{
			Code:      utils.CodeAlreadyExists,
			Message:   message,
			Details:   details,
			Timestamp: time.Now().UTC(),
			RequestID: getRequestID(r),
		},
	}

	json.NewEncoder(w).Encode(errorResponse)
}

func SendInternalError(w http.ResponseWriter, r *http.Request, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)

	errorResponse := ErrorResponse{
		Error: ErrorDetail{
			Code:      utils.CodeInternal,
			Message:   message,
			Timestamp: time.Now().UTC(),
			RequestID: getRequestID(r),
		},
	}

	json.NewEncoder(w).Encode(errorResponse)
}

func handlePanic(w http.ResponseWriter, r *http.Request, err any) {

	fmt.Printf("[ERROR] %s PANIC in %s %s: %v\nStack:\n%s\n",
		time.Now().Format(time.RFC3339),
		r.Method,
		r.RequestURI,
		err,
		debug.Stack(),
	)

	SendInternalError(w, r, "Internal server error")
}

func getRequestID(r *http.Request) string {

	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		return requestID
	}

	if requestID := r.Context().Value("RequestID"); requestID != nil {
		if id, ok := requestID.(string); ok {
			return id
		}
	}

	return ""
}

func HTTPErrorFromAppError(err error) int {
	if appErr, ok := err.(*utils.AppError); ok {
		switch appErr.Code {
		case utils.CodeNotFound:
			return http.StatusNotFound
		case utils.CodeAlreadyExists:
			return http.StatusConflict
		case utils.CodeInvalidInput, utils.CodeValidation:
			return http.StatusBadRequest
		case utils.CodeUnauthorized:
			return http.StatusUnauthorized
		case utils.CodeForbidden:
			return http.StatusForbidden
		case utils.CodeTimeout:
			return http.StatusRequestTimeout
		case utils.CodeConcurrentModification:
			return http.StatusConflict
		case utils.CodeTransactionFailed:
			return http.StatusUnprocessableEntity
		default:
			return http.StatusInternalServerError
		}
	}

	if utils.IsNotFound(err) {
		return http.StatusNotFound
	}
	if utils.IsAlreadyExists(err) {
		return http.StatusConflict
	}
	if utils.IsValidation(err) {
		return http.StatusBadRequest
	}

	return http.StatusInternalServerError
}
