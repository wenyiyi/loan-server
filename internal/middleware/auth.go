package middleware

import (
	"crypto/subtle"
	"strings"

	v1 "LoanService/api/v1"
	"LoanService/internal/model"

	"github.com/gin-gonic/gin"
)

// tokens maps valid bearer tokens to identities.
func Auth(tokens map[string]model.Identity) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authorization := ctx.GetHeader("Authorization")

		parts := strings.Fields(authorization)
		if len(parts) != 2 ||
			!strings.EqualFold(parts[0], "Bearer") {
			v1.HandleError(ctx, v1.ErrUnauthorized)
			return
		}

		var identity model.Identity
		var ok bool
		for token, candidate := range tokens {
			if subtle.ConstantTimeCompare([]byte(token), []byte(parts[1])) == 1 {
				identity, ok = candidate, true
				break
			}
		}
		if !ok {
			v1.HandleError(ctx, v1.ErrUnauthorized)
			return
		}

		ctx.Set("identity", identity)
		ctx.Next()
	}
}

// RequireStaff restricts a route group to authenticated employees.
func RequireStaff() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		identity, ok := GetIdentityFromCtx(ctx)
		if !ok {
			v1.HandleError(ctx, v1.ErrUnauthorized)
			return
		}
		if identity.Role != "staff" {
			v1.HandleError(ctx, v1.ErrForbidden)
			return
		}
		ctx.Next()
	}
}

func RequireInvestor() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		identity, ok := GetIdentityFromCtx(ctx)
		if !ok {
			v1.HandleError(ctx, v1.ErrUnauthorized)
			return
		}
		if identity.Role != "investor" {
			v1.HandleError(ctx, v1.ErrForbidden)
			return
		}
		ctx.Next()
	}
}
