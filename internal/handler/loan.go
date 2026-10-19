package handler

import (
	"LoanService/pkg/log"
	"LoanService/pkg/util"
	"time"

	v1 "LoanService/api/v1"
	"LoanService/internal/middleware"
	"LoanService/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
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

// Create creates a new loan
func (h *LoanHandler) Create(ctx *gin.Context) {
	var req v1.LoanCreateRequest

	// validation
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.WithContext(ctx.Request.Context(), h.logger).Error("invalid create loan request", zap.Error(err))
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	// ROI (investor return) should not exceed Rate (borrower interest rate)
	// The difference represents the platform's interest spread
	if req.Rate.IsNegative() || req.Rate.GreaterThan(decimal.NewFromInt(100)) || req.ROI.IsNegative() || req.ROI.GreaterThan(*req.Rate) || !util.ValidURL(req.AgreementURL, ".pdf") {
		log.WithContext(ctx.Request.Context(), h.logger).Error("ROI cannot exceed rate")
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	loan, err := h.service.Create(ctx.Request.Context(), req)
	if err != nil {
		log.WithContext(ctx.Request.Context(), h.logger).Error("failed to create loan", zap.Error(err))
		v1.HandleError(ctx, err)
		return
	}

	v1.HandleSuccess(ctx, loan)
}

// Approve approves a proposed loan
func (h *LoanHandler) Approve(ctx *gin.Context) {
	var req v1.ApprovalRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Error(ctx.Request.Context(), "loan.Approve failed", err)
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	identity, ok := middleware.GetIdentityFromCtx(ctx)
	if !ok {
		v1.HandleError(ctx, v1.ErrUnauthorized)
		return
	}
	loanNo := ctx.Param("loan_no")
	if err := h.service.Approve(ctx.Request.Context(), loanNo, identity.ID, req); err != nil {
		log.WithContext(ctx.Request.Context(), h.logger).
			Error("failed to approve loan", zap.Error(err))
		v1.HandleError(ctx, err)
		return
	}

	v1.HandleSuccess(ctx, nil)
}

// Disburse disburses a fully invested loan
func (h *LoanHandler) Disburse(ctx *gin.Context) {
	var req v1.DisbursementRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	identity, ok := middleware.GetIdentityFromCtx(ctx)
	if !ok {
		v1.HandleError(ctx, v1.ErrUnauthorized)
		return
	}

	if !util.ValidURL(req.AgreementURL, ".pdf", ".jpg", ".jpeg") ||
		req.At <= 0 ||
		req.At > time.Now().UnixMilli() {
		v1.HandleError(ctx, v1.ErrRequestParams)
		return
	}

	loanNo := ctx.Param("loan_no")

	if err := h.service.Disburse(
		ctx.Request.Context(),
		loanNo,
		identity.ID,
		req,
	); err != nil {
		log.WithContext(ctx.Request.Context(), h.logger).
			Error("failed to disburse loan", zap.Error(err))

		v1.HandleError(ctx, err)
		return
	}

	v1.HandleSuccess(ctx, nil)
}
