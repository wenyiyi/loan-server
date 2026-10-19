package v1

import "net/http"

type Error struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func NewError(status, code int, message string) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status}
}

func (e *Error) Error() string { return e.Message }

var (
	ErrSuccess              = NewError(http.StatusOK, 0, "success")
	ErrRequestParams        = NewError(http.StatusBadRequest, 1000, "params error")
	ErrUnauthorized         = NewError(http.StatusUnauthorized, 1001, "Unauthorized")
	ErrForbidden            = NewError(http.StatusForbidden, 1002, "Forbidden")
	ErrNotFound             = NewError(http.StatusNotFound, 1003, "record not found")
	ErrUnsupportedMediaType = NewError(http.StatusUnsupportedMediaType, 1004, "application/json required")
	ErrState                = NewError(http.StatusConflict, 2001, "operation not allowed in current loan state")
	ErrAmount               = NewError(http.StatusConflict, 2002, "investment exceeds remaining principal")
	ErrIdempotency          = NewError(http.StatusConflict, 2003, "idempotency key already used for a different request")
	ErrInvalidApprovalTime  = NewError(http.StatusBadRequest, 2004, "approval time cannot be earlier than loan creation time")
	ErrConflictError        = NewError(http.StatusConflict, 2005, "Conflict")
	ErrInternalServerError  = NewError(http.StatusInternalServerError, 5000, "Internal Server Error")
	ErrNotReady             = NewError(http.StatusServiceUnavailable, 5001, "database unavailable")
)
