package middleware

import (
	"LoanService/internal/model"

	"github.com/gin-gonic/gin"
)

func GetIdentityFromCtx(ctx *gin.Context) (model.Identity, bool) {
	value, exists := ctx.Get("identity")
	if !exists {
		return model.Identity{}, false
	}

	identity, ok := value.(model.Identity)
	return identity, ok
}
