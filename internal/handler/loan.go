package handler

import (
	v1 "LoanService/api/v1"
	"LoanService/internal/middleware"
	"LoanService/internal/service"
	"LoanService/pkg/log"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type LoanHandler struct {
	*Handler
	service service.Service
}

func NewLoanHandler(
	handler *Handler,
	svc service.Service,
) *LoanHandler {
	return &LoanHandler{
		Handler: handler,
		service: svc,
	}
}

// Create creates a new loan.
func (h *LoanHandler) Create(ctx *gin.Context) {
	logger := log.WithContext(ctx.Request.Context(), h.logger)

	var req v1.LoanCreateRequest

	// 1 Validate request format.
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Error("invalid create loan request", zap.Error(err))
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	// 2 Validate business rules.
	if err := req.Validate(); err != nil {
		logger.Error("invalid loan business parameters", zap.Error(err))
		v1.HandleError(ctx, err)
		return
	}

	// 3 Create loan.
	loan, err := h.service.Create(ctx.Request.Context(), req)
	if err != nil {
		logger.Error("failed to create loan", zap.Error(err))
		v1.HandleError(ctx, err)
		return
	}

	v1.HandleSuccess(ctx, loan)
}

// Approve approves a proposed loan.
func (h *LoanHandler) Approve(ctx *gin.Context) {
	logger := log.WithContext(ctx.Request.Context(), h.logger)
	loanNo := ctx.Param("loan_no")

	var req v1.ApprovalRequest

	// 1. Validate request format.
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Error("invalid approval request", zap.String("loan_no", loanNo), zap.Error(err))
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	// 2. Get authenticated employee identity.
	identity, ok := middleware.GetIdentityFromCtx(ctx)
	if !ok {
		logger.Error("unauthorized loan approval", zap.String("loan_no", loanNo))
		v1.HandleError(ctx, v1.ErrUnauthorized)
		return
	}

	// 3. Approve loan.
	err := h.service.Approve(ctx.Request.Context(), loanNo, identity.ID, req)
	if err != nil {
		logger.Error("failed to approve loan", zap.String("loan_no", loanNo), zap.Error(err))
		v1.HandleError(ctx, err)
		return
	}

	v1.HandleSuccess(ctx, nil)
}

// Disburse disburses a fully invested loan.
func (h *LoanHandler) Disburse(ctx *gin.Context) {
	logger := log.WithContext(ctx.Request.Context(), h.logger)
	loanNo := ctx.Param("loan_no")

	var req v1.DisbursementRequest
	// 1. Validate request format.
	if err := ctx.ShouldBindJSON(&req); err != nil {
		logger.Error("invalid disbursement request", zap.String("loan_no", loanNo), zap.Error(err))
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	// 2. Get authenticated employee identity.
	identity, ok := middleware.GetIdentityFromCtx(ctx)
	if !ok {
		logger.Error("unauthorized loan disbursement", zap.String("loan_no", loanNo))
		v1.HandleError(ctx, v1.ErrUnauthorized)
		return
	}

	// 3. Disburse loan.
	err := h.service.Disburse(ctx.Request.Context(), loanNo, identity.ID, req)
	if err != nil {
		logger.Error("failed to disburse loan", zap.String("loan_no", loanNo), zap.Error(err))
		v1.HandleError(ctx, err)
		return
	}

	v1.HandleSuccess(ctx, nil)
}
