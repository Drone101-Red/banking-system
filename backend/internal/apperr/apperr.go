package apperr

import (
    "encoding/json"
    "errors"
    "log"
    "net/http"
)

type AppError struct {
    Code       string
    Message    string
    HTTPStatus int
    Err        error
}

func (e *AppError) Error() string {
    if e.Err != nil {
        return e.Code + ": " + e.Message + " (" + e.Err.Error() + ")"
    }
    return e.Code + ": " + e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

func BadRequest(code, message string) *AppError {
    return &AppError{Code: code, Message: message, HTTPStatus: http.StatusBadRequest}
}

func Unauthorized(code, message string) *AppError {
    return &AppError{Code: code, Message: message, HTTPStatus: http.StatusUnauthorized}
}

func Forbidden(code, message string) *AppError {
    return &AppError{Code: code, Message: message, HTTPStatus: http.StatusForbidden}
}

func NotFound(code, message string) *AppError {
    return &AppError{Code: code, Message: message, HTTPStatus: http.StatusNotFound}
}

func Conflict(code, message string) *AppError {
    return &AppError{Code: code, Message: message, HTTPStatus: http.StatusConflict}
}

func Internal(err error) *AppError {
    return &AppError{
        Code:       "INTERNAL_ERROR",
        Message:    "Error interno del servidor",
        HTTPStatus: http.StatusInternalServerError,
        Err:        err,
    }
}

type errorBody struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

// Write serializa el AppError a JSON y escribe el HTTP status.
// Si err no es *AppError, escribe 500 con mensaje generico.
func Write(w http.ResponseWriter, err error) {
    var appErr *AppError
    if !errors.As(err, &appErr) {
        log.Printf("[apperr] error no tipado: %v", err)
        appErr = Internal(err)
    }

    if appErr.HTTPStatus >= 500 && appErr.Err != nil {
        log.Printf("[apperr] %s: %v", appErr.Code, appErr.Err)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(appErr.HTTPStatus)
    _ = json.NewEncoder(w).Encode(errorBody{
        Code:    appErr.Code,
        Message: appErr.Message,
    })
}