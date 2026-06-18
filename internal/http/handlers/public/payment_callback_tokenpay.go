package public

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/handlers/shared"

	"github.com/gin-gonic/gin"
)

func (h *Handler) HandleTokenPayCallback(c *gin.Context) bool {
	log := shared.RequestLog(c)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// 轻量级特征检测：Signature + OutOrderId + Id（TokenOrderId）必须同时存在
	var probe struct {
		Signature    string `json:"Signature"`
		OutOrderID   string `json:"OutOrderId"`
		TokenOrderID string `json:"Id"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		log.Debugw("tokenpay_callback_parse_failed", "error", err)
		return false
	}
	if strings.TrimSpace(probe.Signature) == "" || strings.TrimSpace(probe.OutOrderID) == "" || strings.TrimSpace(probe.TokenOrderID) == "" {
		log.Debugw("tokenpay_callback_not_matched")
		return false
	}

	log.Infow("tokenpay_callback_received",
		"out_order_id", probe.OutOrderID,
		"token_order_id", probe.TokenOrderID,
		"raw_body", callbackRawBodyForLog(body),
	)

	payment, err := h.PaymentRepo.GetByGatewayOrderNo(probe.OutOrderID)
	if err != nil || payment == nil {
		payment, err = h.PaymentRepo.GetLatestByProviderRef(probe.TokenOrderID)
		if err != nil || payment == nil {
			log.Warnw("tokenpay_callback_payment_not_found", "out_order_id", probe.OutOrderID, "token_order_id", probe.TokenOrderID, "error", err)
			c.String(200, constants.TokenPayCallbackFail)
			return true
		}
	}

	log.Debugw("tokenpay_callback_payment_found", "payment_id", payment.ID, "channel_id", payment.ChannelID)

	channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
	if err != nil || channel == nil {
		log.Warnw("tokenpay_callback_channel_not_found", "payment_id", payment.ID, "channel_id", payment.ChannelID, "error", err)
		c.String(200, constants.TokenPayCallbackFail)
		return true
	}
	if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderTokenpay {
		log.Warnw("tokenpay_callback_provider_invalid", "provider_type", channel.ProviderType)
		c.String(200, constants.TokenPayCallbackFail)
		return true
	}

	updated, err := h.PaymentService.HandleSyncCallback(channel, nil, body)
	if err != nil {
		log.Errorw("tokenpay_callback_handle_failed", "payment_id", payment.ID, "error", err)
		c.String(200, constants.TokenPayCallbackFail)
		return true
	}

	log.Infow("tokenpay_callback_processed", "payment_id", payment.ID, "status", updated.Status)
	c.String(200, constants.TokenPayCallbackSuccess)
	return true
}
