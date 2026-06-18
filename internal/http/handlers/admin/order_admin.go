package admin

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminOrderListItem 管理端订单列表返回
type AdminOrderListItem struct {
	models.Order
	UserEmail       string `json:"user_email,omitempty"`
	UserDisplayName string `json:"user_display_name,omitempty"`
}

// AdminOrderDetail 管理端订单详情返回
type AdminOrderDetail struct {
	models.Order
	UserEmail       string             `json:"user_email,omitempty"`
	UserDisplayName string             `json:"user_display_name,omitempty"`
	CouponCode      string             `json:"coupon_code,omitempty"`
	PromotionName   string             `json:"promotion_name,omitempty"`
	Payments        []AdminPaymentItem `json:"payments,omitempty"`
}

// AdminListOrders 管理端订单列表
func (h *Handler) AdminListOrders(c *gin.Context) {
	page, pageSize := shared.ParsePagination(c)

	status := strings.TrimSpace(c.Query("status"))
	userIDRaw := c.Query("user_id")
	userKeyword := strings.TrimSpace(c.Query("user_keyword"))
	orderNo := strings.TrimSpace(c.Query("order_no"))
	guestEmail := strings.TrimSpace(c.Query("guest_email"))

	createdFrom, createdTo, err := shared.ParseQueryTimeRange(c, "created_from", "created_to")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	var userID uint
	userID, _ = shared.ParseQueryUint(userIDRaw, false)

	productKeyword := strings.TrimSpace(c.Query("product_keyword"))
	sortBy := strings.TrimSpace(c.Query("sort_by"))
	sortOrder := strings.TrimSpace(c.Query("sort_order"))

	orders, total, err := h.OrderService.ListOrdersForAdmin(repository.OrderListFilter{
		Page:           page,
		PageSize:       pageSize,
		UserID:         userID,
		UserKeyword:    userKeyword,
		Status:         status,
		OrderNo:        orderNo,
		GuestEmail:     guestEmail,
		ProductKeyword: productKeyword,
		CreatedFrom:    createdFrom,
		CreatedTo:      createdTo,
		SortBy:         sortBy,
		SortOrder:      sortOrder,
	})
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	userMap := map[uint]models.User{}
	userIDs := make([]uint, 0, len(orders))
	seen := map[uint]struct{}{}
	for _, order := range orders {
		if order.UserID == 0 {
			continue
		}
		if _, ok := seen[order.UserID]; ok {
			continue
		}
		seen[order.UserID] = struct{}{}
		userIDs = append(userIDs, order.UserID)
	}
	if len(userIDs) > 0 {
		users, err := h.UserRepo.ListByIDs(userIDs)
		if err != nil {
			shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		for _, user := range users {
			userMap[user.ID] = user
		}
	}

	items := make([]AdminOrderListItem, 0, len(orders))
	for _, order := range orders {
		var email, displayName string
		if user, ok := userMap[order.UserID]; ok {
			email = user.Email
			displayName = user.DisplayName
		}
		items = append(items, AdminOrderListItem{
			Order:           order,
			UserEmail:       email,
			UserDisplayName: displayName,
		})
	}

	response.SuccessWithPage(c, items, pagination)
}

// AdminGetOrder 管理端订单详情
func (h *Handler) AdminGetOrder(c *gin.Context) {
	orderID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	order, err := h.OrderService.GetOrderForAdmin(orderID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		}
		return
	}
	var email, displayName string
	if order.UserID != 0 {
		user, err := h.UserRepo.GetByID(order.UserID)
		if err != nil {
			shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		if user != nil {
			email = user.Email
			displayName = user.DisplayName
		}
	}

	var couponCode string
	if order.CouponID != nil && *order.CouponID > 0 {
		coupon, err := h.CouponRepo.GetByID(*order.CouponID)
		if err != nil {
			shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		if coupon != nil {
			couponCode = coupon.Code
		}
	}

	var promotionName string
	if order.PromotionID != nil && *order.PromotionID > 0 {
		promotion, err := h.PromotionRepo.GetByID(*order.PromotionID)
		if err != nil {
			shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		if promotion != nil {
			promotionName = promotion.Name
		}
	}

	promotionNameMap := make(map[uint]string)
	for i := range order.Items {
		item := order.Items[i]
		if item.PromotionID == nil || *item.PromotionID == 0 {
			continue
		}
		promotionID := *item.PromotionID
		if _, ok := promotionNameMap[promotionID]; ok {
			continue
		}
		promotion, err := h.PromotionRepo.GetByID(promotionID)
		if err != nil {
			shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
			return
		}
		if promotion != nil {
			promotionNameMap[promotionID] = promotion.Name
		} else {
			promotionNameMap[promotionID] = ""
		}
	}
	for i := range order.Children {
		for _, item := range order.Children[i].Items {
			if item.PromotionID == nil || *item.PromotionID == 0 {
				continue
			}
			promotionID := *item.PromotionID
			if _, ok := promotionNameMap[promotionID]; ok {
				continue
			}
			promotion, err := h.PromotionRepo.GetByID(promotionID)
			if err != nil {
				shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
				return
			}
			if promotion != nil {
				promotionNameMap[promotionID] = promotion.Name
			} else {
				promotionNameMap[promotionID] = ""
			}
		}
	}
	for i := range order.Items {
		item := &order.Items[i]
		if item.PromotionID == nil || *item.PromotionID == 0 {
			continue
		}
		item.PromotionName = promotionNameMap[*item.PromotionID]
	}
	for i := range order.Children {
		for j := range order.Children[i].Items {
			item := &order.Children[i].Items[j]
			if item.PromotionID == nil || *item.PromotionID == 0 {
				continue
			}
			item.PromotionName = promotionNameMap[*item.PromotionID]
		}
	}

	payments, err := h.PaymentRepo.ListByOrderID(order.ID)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}
	channelNameMap, err := h.resolvePaymentChannelNames(payments)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}
	paymentItems := make([]AdminPaymentItem, 0, len(payments))
	for _, payment := range payments {
		paymentItems = append(paymentItems, AdminPaymentItem{
			Payment:     payment,
			ChannelName: channelNameMap[payment.ChannelID],
		})
	}

	order.TruncateFulfillmentPayload()
	response.Success(c, AdminOrderDetail{
		Order:           *order,
		UserEmail:       email,
		UserDisplayName: displayName,
		CouponCode:      couponCode,
		PromotionName:   promotionName,
		Payments:        paymentItems,
	})
}

// AdminUpdateOrderStatusRequest 管理端更新订单状态请求
type AdminUpdateOrderStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// AdminUpdateOrderStatus 管理端更新订单状态
func (h *Handler) AdminUpdateOrderStatus(c *gin.Context) {
	orderID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}

	var req AdminUpdateOrderStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	order, err := h.OrderService.UpdateOrderStatus(orderID, req.Status)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
		case errors.Is(err, service.ErrOrderStatusInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.order_update_failed", err)
		}
		return
	}

	response.Success(c, order)
}

// AdminDownloadFulfillment 管理端下载订单交付内容
func (h *Handler) AdminDownloadFulfillment(c *gin.Context) {
	orderID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
		return
	}
	order, err := h.OrderService.GetOrderForAdmin(orderID)
	if err != nil || order == nil {
		shared.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
		return
	}
	payload := collectAdminFulfillmentPayload(order)
	if payload == "" {
		shared.RespondError(c, response.CodeNotFound, "error.fulfillment_not_found", nil)
		return
	}
	filename := "fulfillment-" + order.OrderNo + ".txt"
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Data(200, "text/plain; charset=utf-8", []byte(payload))
}

func collectAdminFulfillmentPayload(order *models.Order) string {
	if order.Fulfillment != nil && order.Fulfillment.Payload != "" {
		return order.Fulfillment.Payload
	}
	var parts []string
	for _, child := range order.Children {
		if child.Fulfillment != nil && child.Fulfillment.Payload != "" {
			parts = append(parts, child.Fulfillment.Payload)
		}
	}
	return strings.Join(parts, "\n")
}
