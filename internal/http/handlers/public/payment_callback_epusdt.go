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

// HandleEpusdtCallback 处理真 epusdt（GMPay）回调。
// 特征：JSON body 必须同时含 pid + trade_id + order_id（pid 是与 BEpusdt 的强区分）。
// 不匹配特征则返回 false 让链向后传给 BEpusdt handler。
func (h *Handler) HandleEpusdtCallback(c *gin.Context) bool {
	log := shared.RequestLog(c)

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}
	// 恢复请求体供后续 handler 重读
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// 轻量级特征检测：pid + trade_id + order_id 必须同时存在
	var probe struct {
		PID     string `json:"pid"`
		TradeID string `json:"trade_id"`
		OrderID string `json:"order_id"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		log.Debugw("epusdt_callback_parse_failed", "error", err)
		return false
	}
	if strings.TrimSpace(probe.PID) == "" || probe.TradeID == "" || probe.OrderID == "" {
		log.Debugw("epusdt_callback_feature_missing",
			"has_pid", strings.TrimSpace(probe.PID) != "",
			"has_trade_id", probe.TradeID != "",
			"has_order_id", probe.OrderID != "")
		return false
	}

	log.Infow("epusdt_callback_received",
		"pid", probe.PID,
		"trade_id", probe.TradeID,
		"order_id", probe.OrderID,
		"raw_body", callbackRawBodyForLog(body),
	)

	// 通过 order_id（我方网关订单号）查 payment，降级到 trade_id（第三方流水号）
	payment, err := h.PaymentRepo.GetByGatewayOrderNo(probe.OrderID)
	if err != nil || payment == nil {
		payment, err = h.PaymentRepo.GetLatestByProviderRef(probe.TradeID)
		if err != nil || payment == nil {
			log.Warnw("epusdt_callback_payment_not_found", "order_id", probe.OrderID, "trade_id", probe.TradeID, "error", err)
			c.String(200, constants.EpusdtCallbackFail)
			return true
		}
	}

	log.Debugw("epusdt_callback_payment_found", "payment_id", payment.ID, "channel_id", payment.ChannelID)

	// 获取支付渠道
	channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
	if err != nil || channel == nil {
		log.Warnw("epusdt_callback_channel_not_found", "channel_id", payment.ChannelID, "error", err)
		c.String(200, constants.EpusdtCallbackFail)
		return true
	}

	// 验证是否为 epusdt 渠道
	if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderEpusdt {
		log.Warnw("epusdt_callback_invalid_provider", "provider_type", channel.ProviderType)
		c.String(200, constants.EpusdtCallbackFail)
		return true
	}

	updated, err := h.PaymentService.HandleSyncCallback(channel, nil, body)
	if err != nil {
		log.Errorw("epusdt_callback_handle_failed", "payment_id", payment.ID, "error", err)
		c.String(200, constants.EpusdtCallbackFail)
		return true
	}

	log.Infow("epusdt_callback_processed", "payment_id", payment.ID, "status", updated.Status)
	c.String(200, constants.EpusdtCallbackSuccess) // "ok"
	return true
}
