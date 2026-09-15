package errors

import (
	"errors"
	"fmt"
	"net/http"
)

type Code int

const (
	CodeOK             Code = 2000
	CodeBadRequest     Code = 4000
	CodeUnauthorized   Code = 4010
	CodeTokenExpired   Code = 4011
	CodeForbidden      Code = 4030
	CodeNotFound       Code = 4040
	CodeConflict       Code = 4090
	CodeValidation     Code = 4220
	CodeInternalServer Code = 5000
)

func (c Code) HTTPStatus() int {
	switch c {
	case CodeOK:
		return http.StatusOK
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized, CodeTokenExpired:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeValidation:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// AppError is a domain error that carries both an application code and a user-facing message.
type AppError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func Wrap(code Code, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

// Convenience constructors

func BadRequest(msg string) *AppError              { return New(CodeBadRequest, msg) }
func Unauthorized(msg string) *AppError            { return New(CodeUnauthorized, msg) }
func Forbidden(msg string) *AppError               { return New(CodeForbidden, msg) }
func NotFound(msg string) *AppError                { return New(CodeNotFound, msg) }
func Conflict(msg string) *AppError                { return New(CodeConflict, msg) }
func Validation(msg string) *AppError              { return New(CodeValidation, msg) }
func Internal(msg string) *AppError                { return New(CodeInternalServer, msg) }
func InternalWrap(msg string, err error) *AppError { return Wrap(CodeInternalServer, msg, err) }

// AsAppError attempts to extract an AppError from an error chain.
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
