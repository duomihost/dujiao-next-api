package admin

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"
)

// AffiliateProfileStatusRequest 返利用户状态更新请求
type AffiliateProfileStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// BatchAffiliateProfileStatusRequest 返利用户批量状态更新请求
type BatchAffiliateProfileStatusRequest struct {
	ProfileIDs []uint `json:"profile_ids" binding:"required"`
	Status     string `json:"status" binding:"required"`
}

// ListAffiliateUsers 管理端推广用户列表
func (h *Handler) ListAffiliateUsers(c *gin.Context) {
	if h.AffiliateService == nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	page, pageSize := shared.ParsePagination(c)
	userID, _ := shared.ParseQueryUint(c.Query("user_id"), false)

	rows, total, err := h.AffiliateService.ListAdminUsers(repository.AffiliateProfileListFilter{
		Page:     page,
		PageSize: pageSize,
		UserID:   userID,
		Status:   strings.TrimSpace(c.Query("status")),
		Code:     strings.TrimSpace(c.Query("code")),
		Keyword:  strings.TrimSpace(c.Query("keyword")),
	})
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, rows, response.BuildPagination(page, pageSize, total))
}

// ListAffiliateCommissions 管理端佣金列表
func (h *Handler) ListAffiliateCommissions(c *gin.Context) {
	if h.AffiliateService == nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	page, pageSize := shared.ParsePagination(c)
	profileID, _ := shared.ParseQueryUint(c.Query("affiliate_profile_id"), false)

	rows, total, err := h.AffiliateService.ListAdminCommissions(service.AffiliateAdminCommissionListFilter{
		Page:               page,
		PageSize:           pageSize,
		AffiliateProfileID: profileID,
		OrderNo:            strings.TrimSpace(c.Query("order_no")),
		Status:             strings.TrimSpace(c.Query("status")),
		Keyword:            strings.TrimSpace(c.Query("keyword")),
	})
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, rows, response.BuildPagination(page, pageSize, total))
}

// ListAffiliateWithdraws 管理端提现审核列表
func (h *Handler) ListAffiliateWithdraws(c *gin.Context) {
	if h.AffiliateService == nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	page, pageSize := shared.ParsePagination(c)
	profileID, _ := shared.ParseQueryUint(c.Query("affiliate_profile_id"), false)

	rows, total, err := h.AffiliateService.ListAdminWithdraws(service.AffiliateAdminWithdrawListFilter{
		Page:               page,
		PageSize:           pageSize,
		AffiliateProfileID: profileID,
		Status:             strings.TrimSpace(c.Query("status")),
		Keyword:            strings.TrimSpace(c.Query("keyword")),
	})
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, rows, response.BuildPagination(page, pageSize, total))
}

// UpdateAffiliateUserStatus 管理端更新返利用户状态
func (h *Handler) UpdateAffiliateUserStatus(c *gin.Context) {
	if h.AffiliateService == nil {
		shared.RespondError(c, response.CodeInternal, "error.save_failed", nil)
		return
	}
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	var req AffiliateProfileStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	row, err := h.AffiliateService.UpdateAffiliateProfileStatus(id, strings.TrimSpace(req.Status))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.bad_request", nil)
		case errors.Is(err, service.ErrAffiliateProfileStatusInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.save_failed", err)
		}
		return
	}
	response.Success(c, row)
}

// BatchUpdateAffiliateUserStatus 管理端批量更新返利用户状态
func (h *Handler) BatchUpdateAffiliateUserStatus(c *gin.Context) {
	if h.AffiliateService == nil {
		shared.RespondError(c, response.CodeInternal, "error.save_failed", nil)
		return
	}
	var req BatchAffiliateProfileStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	if len(req.ProfileIDs) == 0 {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	updated, err := h.AffiliateService.BatchUpdateAffiliateProfileStatus(req.ProfileIDs, strings.TrimSpace(req.Status))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAffiliateProfileStatusInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.save_failed", err)
		}
		return
	}
	response.Success(c, gin.H{"updated": updated})
}

// AffiliateReviewWithdrawRequest 提现审核请求
type AffiliateReviewWithdrawRequest struct {
	Reason string `json:"reason"`
}

// RejectAffiliateWithdraw 拒绝提现申请
func (h *Handler) RejectAffiliateWithdraw(c *gin.Context) {
	adminID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	if h.AffiliateService == nil {
		shared.RespondError(c, response.CodeInternal, "error.save_failed", nil)
		return
	}
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	var req AffiliateReviewWithdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	row, err := h.AffiliateService.ReviewWithdraw(adminID, id, constants.AffiliateWithdrawActionReject, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.bad_request", nil)
		case errors.Is(err, service.ErrAffiliateWithdrawStatusInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.save_failed", err)
		}
		return
	}
	response.Success(c, row)
}

// PayAffiliateWithdraw 标记提现已支付
func (h *Handler) PayAffiliateWithdraw(c *gin.Context) {
	adminID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	if h.AffiliateService == nil {
		shared.RespondError(c, response.CodeInternal, "error.save_failed", nil)
		return
	}
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	row, err := h.AffiliateService.ReviewWithdraw(adminID, id, constants.AffiliateWithdrawActionPay, "")
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.bad_request", nil)
		case errors.Is(err, service.ErrAffiliateWithdrawStatusInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.save_failed", err)
		}
		return
	}
	response.Success(c, row)
}
