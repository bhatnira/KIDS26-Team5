package apperr

import "net/http"

// AppError is a structured error that carries an HTTP status, an application-level
// code, and a human-readable message. Services return *AppError so handlers can
// map errors to HTTP responses without duplicating status-code logic.
type AppError struct {
	HTTPStatus int
	Code       int
	Msg        string
}

func (e *AppError) Error() string { return e.Msg }

func BadRequest(code int, msg string) *AppError {
	return &AppError{HTTPStatus: http.StatusBadRequest, Code: code, Msg: msg}
}

func CheckFail(code int, msg string) *AppError {
	return &AppError{HTTPStatus: http.StatusUnprocessableEntity, Code: code, Msg: msg}
}

func ServerError(msg string) *AppError {
	return &AppError{HTTPStatus: http.StatusInternalServerError, Code: 5000, Msg: msg}
}

func NotFound(msg string) *AppError {
	return &AppError{HTTPStatus: http.StatusNotFound, Code: 4040, Msg: msg}
}

func Unauthorized(msg string) *AppError {
	return &AppError{HTTPStatus: http.StatusUnauthorized, Code: 4010, Msg: msg}
}

func Forbidden(msg string) *AppError {
	return &AppError{HTTPStatus: http.StatusForbidden, Code: 4030, Msg: msg}
}
