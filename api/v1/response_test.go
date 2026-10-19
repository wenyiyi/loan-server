package v1

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleErrorMapsStatusAndBusinessCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		err          *Error
		status, code int
	}{
		{ErrRequestParams, 400, 1000}, {ErrUnauthorized, 401, 1001},
		{ErrForbidden, 403, 1002}, {ErrNotFound, 404, 1003},
		{ErrUnsupportedMediaType, 415, 1004}, {ErrState, 409, 2001},
		{ErrAmount, 409, 2002}, {ErrIdempotency, 409, 2003},
		{ErrInvalidApprovalTime, 400, 2004}, {ErrConflictError, 409, 2005},
		{ErrInternalServerError, 500, 5000}, {ErrNotReady, 503, 5001},
	} {
		t.Run(test.err.Message, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
			HandleError(ctx, fmt.Errorf("operation failed: %w", test.err))
			require.Equal(t, test.status, response.Code)
			require.JSONEq(t, fmt.Sprintf(`{"code":%d,"message":%q}`, test.code, test.err.Message), response.Body.String())
			require.Equal(t, test.code, ctx.GetInt("error_code"))
			require.True(t, ctx.IsAborted())
		})
	}
}

func TestHandleErrorHidesInternalDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var typedNil *Error
	for _, err := range []error{errors.New("private database password"), nil, typedNil} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
		HandleError(ctx, err)
		require.Equal(t, http.StatusInternalServerError, response.Code)
		require.JSONEq(t, `{"code":5000,"message":"Internal Server Error"}`, response.Body.String())
	}
}
