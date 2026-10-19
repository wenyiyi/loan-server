package handler

import (
	"LoanService/pkg/log"

	"go.uber.org/zap"

	v1 "LoanService/api/v1"
	"LoanService/internal/middleware"
	"LoanService/internal/service"

	"github.com/gin-gonic/gin"
)

type InvestmentHandler struct {
	*Handler
	service service.Service
}

func NewInvestmentHandler(
	handler *Handler,
	svc service.Service,
) *InvestmentHandler {
	return &InvestmentHandler{
		Handler: handler,
		service: svc,
	}
}

// Invest creates a new investment for a loan.
func (h *InvestmentHandler) Invest(ctx *gin.Context) {
	var req v1.InvestmentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.WithContext(ctx.Request.Context(), h.logger).Error("invalid investment request", zap.Error(err))
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	key := ctx.GetHeader("Idempotency-Key")
	if len(key) == 0 || len(key) > 255 {
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	identity, ok := middleware.GetIdentityFromCtx(ctx)
	if !ok {
		v1.HandleError(ctx, v1.ErrUnauthorized)
		return
	}

	req.Email = identity.Email
	req.Key = key

	loanNo := ctx.Param(`loan_no`)
	result, err := h.service.Invest(ctx.Request.Context(), loanNo, identity.ID, req)
	if err != nil {
		log.WithContext(ctx.Request.Context(), h.logger).Error("failed to create investment", zap.Error(err))
		v1.HandleError(ctx, err)
		return
	}

	v1.HandleSuccess(ctx, result)
}
