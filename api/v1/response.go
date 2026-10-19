package v1

import (
	"errors"
	"net/http"

	"LoanService/pkg/log"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func HandleSuccess(ctx *gin.Context, data any) {
	handleSuccess(ctx, http.StatusOK, data)
}

func handleSuccess(ctx *gin.Context, status int, data any) {
	if data == nil {
		data = gin.H{}
	}
	ctx.JSON(status, Response{Code: ErrSuccess.Code, Message: ErrSuccess.Message, Data: data})
}

// HandleError maps public errors to their HTTP status and hides internal details.
func HandleError(ctx *gin.Context, err error) {
	var apiError *Error
	if !errors.As(err, &apiError) || apiError == nil {
		log.Error(ctx.Request.Context(), "unhandled API error", err)
		apiError = ErrInternalServerError
	}
	ctx.Set("error_code", apiError.Code)
	ctx.AbortWithStatusJSON(apiError.HTTPStatus, apiError)
}

func HandleInternalError(ctx *gin.Context) {
	HandleError(ctx, ErrInternalServerError)
}
