package admin

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminRefundOrderToWalletRequest 管理端订单退款到余额请求
type AdminRefundOrderToWalletRequest struct {
	Amount string `json:"amount" binding:"required"`
	Remark string `json:"remark"`
}

// AdminManualRefundOrderRequest 管理端手动退款请求（不处理钱包/支付渠道）
type AdminManualRefundOrderRequest struct {
	Amount string `json:"amount" binding:"required"`
	Remark string `json:"remark"`
}

// GetAdminOrderRefunds 获取管理端退款记录列表
func (h *Handler) GetAdminOrderRefunds(c *gin.Context) {
	page, pageSize := shared.ParsePagination(c)

	items, total, err := h.OrderRefundService.ListAdminRefundItems(service.AdminOrderRefundListQuery{
		Page:           page,
		PageSize:       pageSize,
		UserID:         c.Query("user_id"),
		UserKeyword:    c.Query("user_keyword"),
		OrderNo:        c.Query("order_no"),
		GuestEmail:     c.Query("guest_email"),
		ProductKeyword: c.Query("product_keyword"),
		ProductName:    c.Query("product_name"),
		CreatedFrom:    c.Query("created_from"),
		CreatedTo:      c.Query("created_to"),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderFetchFailed):
			shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		default:
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		}
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, items, pagination)
}

// GetAdminOrderRefund 获取管理端退款记录详情
func (h *Handler) GetAdminOrderRefund(c *gin.Context) {
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	item, err := h.OrderRefundService.GetAdminRefundItem(id)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		}
		return
	}
	response.Success(c, item)
}

// AdminRefundOrderToWallet 管理端订单退款到余额
func (h *Handler) AdminRefundOrderToWallet(c *gin.Context) {
	orderID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}
	var req AdminRefundOrderToWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	amount, err := h.OrderRefundService.ParseRefundAmount(req.Amount)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	order, txn, refundRecord, err := h.WalletService.AdminRefundToWallet(service.AdminRefundToWalletInput{
		OrderID: orderID,
		Amount:  amount,
		Remark:  req.Remark,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
		case errors.Is(err, service.ErrOrderStatusInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		case errors.Is(err, service.ErrOrderRefundExpired):
			shared.RespondError(c, response.CodeBadRequest, "error.order_refund_expired", nil)
		case errors.Is(err, service.ErrWalletInvalidAmount), errors.Is(err, service.ErrWalletRefundExceeded), errors.Is(err, service.ErrWalletNotSupportedForGuest):
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}
	h.enqueueOrderRefundStatusEmail(order, refundRecord)

	response.Success(c, gin.H{
		"order":       order,
		"transaction": txn,
	})
}

// AdminManualRefundOrder 管理端手动退款（不处理钱包/支付渠道）
func (h *Handler) AdminManualRefundOrder(c *gin.Context) {
	orderID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}
	var req AdminManualRefundOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	amount, err := h.OrderRefundService.ParseRefundAmount(req.Amount)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	order, refundRecord, err := h.OrderRefundService.AdminManualRefund(service.AdminManualRefundInput{
		OrderID: orderID,
		Amount:  amount,
		Remark:  req.Remark,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
		case errors.Is(err, service.ErrOrderStatusInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		case errors.Is(err, service.ErrOrderRefundExpired):
			shared.RespondError(c, response.CodeBadRequest, "error.order_refund_expired", nil)
		case errors.Is(err, service.ErrWalletInvalidAmount), errors.Is(err, service.ErrWalletRefundExceeded):
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}
	h.enqueueOrderRefundStatusEmail(order, refundRecord)

	response.Success(c, gin.H{
		"order": order,
	})
}

// enqueueOrderRefundStatusEmail 异步发送退款后的订单状态邮件（优先父订单维度）。
func (h *Handler) enqueueOrderRefundStatusEmail(order *models.Order, refundRecord *models.OrderRefundRecord) {
	if h == nil || order == nil || h.QueueClient == nil {
		return
	}
	targetOrder := order
	if order.ParentID != nil && *order.ParentID > 0 {
		parentOrder, err := h.OrderRepo.GetByID(*order.ParentID)
		if err != nil {
			logger.Warnw("admin_order_refund_load_parent_failed",
				"order_id", order.ID,
				"parent_id", *order.ParentID,
				"error", err,
			)
		} else if parentOrder != nil {
			targetOrder = parentOrder
		}
	}
	if targetOrder.ID == 0 {
		return
	}
	status := strings.TrimSpace(targetOrder.Status)
	if status == "" {
		return
	}
	if err := h.QueueClient.EnqueueOrderStatusEmail(queue.OrderStatusEmailPayload{
		OrderID:        targetOrder.ID,
		Status:         status,
		RefundRecordID: resolveRefundRecordID(refundRecord),
	}); err != nil {
		logger.Warnw("admin_order_refund_enqueue_status_email_failed",
			"order_id", targetOrder.ID,
			"status", status,
			"error", err,
		)
	}
}

// resolveRefundRecordID 安全读取退款记录ID。
func resolveRefundRecordID(record *models.OrderRefundRecord) uint {
	if record == nil {
		return 0
	}
	return record.ID
}
