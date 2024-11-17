// Copyright (c) 2024 TruthOS
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package types

import "fmt"

// ErrorType represents different categories of errors
type ErrorType string

const (
	ErrConfiguration ErrorType = "configuration_error"
	ErrExecution     ErrorType = "execution_error"
	ErrPermission    ErrorType = "permission_error"
	ErrNetwork       ErrorType = "network_error"
	ErrTimeout       ErrorType = "timeout_error"
	ErrValidation    ErrorType = "validation_error"
	ErrRateLimit     ErrorType = "rate_limit_error"
	ErrModel         ErrorType = "model_error"
	ErrSystem        ErrorType = "system_error"
	ErrInput         ErrorType = "input_error"
)

// CustomError provides detailed error information
type CustomError struct {
	Type    ErrorType
	Message string
	Cause   error
}

// Error implements the error interface
func (e *CustomError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// NewCustomError creates a new CustomError
func NewCustomError(errType ErrorType, message string, cause error) *CustomError {
	return &CustomError{
		Type:    errType,
		Message: message,
		Cause:   cause,
	}
}

// IsErrorType checks if an error is of a specific type
func IsErrorType(err error, errType ErrorType) bool {
	if customErr, ok := err.(*CustomError); ok {
		return customErr.Type == errType
	}
	return false
}

// Error formatting helpers
func ErrConfigurationf(format string, args ...interface{}) error {
	return NewCustomError(ErrConfiguration, fmt.Sprintf(format, args...), nil)
}

func ErrExecutionf(format string, args ...interface{}) error {
	return NewCustomError(ErrExecution, fmt.Sprintf(format, args...), nil)
}

func ErrPermissionf(format string, args ...interface{}) error {
	return NewCustomError(ErrPermission, fmt.Sprintf(format, args...), nil)
}

func ErrNetworkf(format string, args ...interface{}) error {
	return NewCustomError(ErrNetwork, fmt.Sprintf(format, args...), nil)
}

func ErrTimeoutf(format string, args ...interface{}) error {
	return NewCustomError(ErrTimeout, fmt.Sprintf(format, args...), nil)
}

func ErrValidationf(format string, args ...interface{}) error {
	return NewCustomError(ErrValidation, fmt.Sprintf(format, args...), nil)
}

func ErrRateLimitf(format string, args ...interface{}) error {
	return NewCustomError(ErrRateLimit, fmt.Sprintf(format, args...), nil)
}

func ErrModelf(format string, args ...interface{}) error {
	return NewCustomError(ErrModel, fmt.Sprintf(format, args...), nil)
}

func ErrSystemf(format string, args ...interface{}) error {
	return NewCustomError(ErrSystem, fmt.Sprintf(format, args...), nil)
}

func ErrInputf(format string, args ...interface{}) error {
	return NewCustomError(ErrInput, fmt.Sprintf(format, args...), nil)
}
