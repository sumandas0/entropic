package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/entropic/entropic/pkg/utils"
)

// ErrorResponse represents a standardized API error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains detailed error information
type ErrorDetail struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	RequestID string                 `json:"request_id,omitempty"`
}

// ErrorHandler returns an error handling middleware
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

// SendError sends a standardized error response
func SendError(w http.ResponseWriter, r *http.Request, err error, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	var errorResponse ErrorResponse
	
	// Check if it's an AppError with structured information
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
		// Generic error
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

// SendValidationError sends a validation error response
func SendValidationError(w http.ResponseWriter, r *http.Request, message string, details map[string]interface{}) {
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

// SendNotFoundError sends a not found error response
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

// SendConflictError sends a conflict error response
func SendConflictError(w http.ResponseWriter, r *http.Request, message string, details map[string]interface{}) {
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

// SendInternalError sends an internal server error response
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

// handlePanic handles panic recovery
func handlePanic(w http.ResponseWriter, r *http.Request, err interface{}) {
	// Log the panic
	fmt.Printf("[ERROR] %s PANIC in %s %s: %v\nStack:\n%s\n",
		time.Now().Format(time.RFC3339),
		r.Method,
		r.RequestURI,
		err,
		debug.Stack(),
	)

	// Send error response
	SendInternalError(w, r, "Internal server error")
}

// getRequestID extracts the request ID from context or headers
func getRequestID(r *http.Request) string {
	// Try to get from Chi middleware context
	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		return requestID
	}
	
	// Try to get from context (Chi middleware sets this)
	if requestID := r.Context().Value("RequestID"); requestID != nil {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	
	return ""
}

// HTTPErrorFromAppError converts an AppError to appropriate HTTP status code
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
	
	// Check for common error types
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