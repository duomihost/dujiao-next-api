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

// HandleBepusdtCallback 处理 BEpusdt 回调
func (h *Handler) HandleBepusdtCallback(c *gin.Context) bool {
	log := shared.RequestLog(c)

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}
	// 恢复请求体供后续使用
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// 轻量级特征检测：trade_id + order_id 必须同时存在（BEpusdt 回调特征，无 pid）
	var probe struct {
		TradeID string `json:"trade_id"`
		OrderID string `json:"order_id"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		log.Debugw("bepusdt_callback_parse_failed", "error", err)
		return false
	}
	if probe.TradeID == "" || probe.OrderID == "" {
		log.Debugw("bepusdt_callback_missing_fields", "trade_id", probe.TradeID, "order_id", probe.OrderID)
		return false
	}

	log.Infow("bepusdt_callback_received",
		"trade_id", probe.TradeID,
		"order_id", probe.OrderID,
		"raw_body", callbackRawBodyForLog(body),
	)

	// 通过 order_id（我方网关订单号）查找支付记录，降级到 trade_id（第三方流水号）
	payment, err := h.PaymentRepo.GetByGatewayOrderNo(probe.OrderID)
	if err != nil || payment == nil {
		payment, err = h.PaymentRepo.GetLatestByProviderRef(probe.TradeID)
		if err != nil || payment == nil {
			log.Warnw("bepusdt_callback_payment_not_found", "order_id", probe.OrderID, "trade_id", probe.TradeID, "error", err)
			c.String(200, constants.BepusdtCallbackFail)
			return true
		}
	}

	log.Debugw("bepusdt_callback_payment_found", "payment_id", payment.ID, "channel_id", payment.ChannelID)

	// 获取支付渠道
	channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
	if err != nil || channel == nil {
		log.Warnw("bepusdt_callback_channel_not_found", "channel_id", payment.ChannelID, "error", err)
		c.String(200, constants.BepusdtCallbackFail)
		return true
	}

	// 验证是否为 BEpusdt 渠道
	if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderBepusdt {
		log.Warnw("bepusdt_callback_invalid_provider", "provider_type", channel.ProviderType)
		c.String(200, constants.BepusdtCallbackFail)
		return true
	}

	updated, err := h.PaymentService.HandleSyncCallback(channel, nil, body)
	if err != nil {
		log.Errorw("bepusdt_callback_handle_failed", "payment_id", payment.ID, "error", err)
		c.String(200, constants.BepusdtCallbackFail)
		return true
	}

	log.Infow("bepusdt_callback_processed", "payment_id", payment.ID, "status", updated.Status)
	c.String(200, constants.BepusdtCallbackSuccess)
	return true
}
