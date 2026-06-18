package public

import (
	"fmt"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) HandleEpayCallback(c *gin.Context) bool {
	log := shared.RequestLog(c)
	form, err := parseCallbackForm(c)
	if err != nil {
		log.Warnw("epay_callback_form_parse_failed", "error", err)
		return false
	}
	outTradeNo := strings.TrimSpace(getFirstValue(form, "out_trade_no"))
	pid := strings.TrimSpace(getFirstValue(form, "pid"))
	if pid == "" || outTradeNo == "" {
		log.Debugw("epay_callback_not_matched", "reason", "missing_pid_or_out_trade_no")
		return false
	}
	if strings.TrimSpace(getFirstValue(form, "trade_status")) == "" {
		log.Debugw("epay_callback_not_matched", "reason", "missing_trade_status")
		return false
	}
	log.Infow("epay_callback_received",
		"client_ip", c.ClientIP(),
		"out_trade_no", outTradeNo,
		"trade_no", strings.TrimSpace(getFirstValue(form, "trade_no")),
		"trade_status", strings.TrimSpace(getFirstValue(form, "trade_status")),
		"raw_form", callbackRawFormForLog(form),
	)
	payment, err := h.PaymentRepo.GetByGatewayOrderNo(outTradeNo)
	if err != nil || payment == nil {
		log.Warnw("epay_callback_payment_not_found", "out_trade_no", outTradeNo, "error", err)
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
	if err != nil || channel == nil {
		log.Warnw("epay_callback_channel_not_found",
			"payment_id", payment.ID,
			"channel_id", payment.ChannelID,
			"error", err,
		)
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderEpay {
		log.Warnw("epay_callback_provider_invalid",
			"payment_id", payment.ID,
			"channel_id", channel.ID,
			"provider_type", channel.ProviderType,
		)
		c.String(200, constants.EpayCallbackFail)
		return true
	}

	updated, err := h.PaymentService.HandleSyncCallback(channel, form, nil)
	if err != nil {
		log.Warnw("epay_callback_handle_failed",
			"payment_id", payment.ID,
			"channel_id", channel.ID,
			"out_trade_no", outTradeNo,
			"error", err,
		)
		h.enqueuePaymentExceptionAlert(c, models.JSON{
			"alert_type":   "epay_callback_handle_failed",
			"alert_level":  "error",
			"payment_id":   fmt.Sprintf("%d", payment.ID),
			"out_trade_no": outTradeNo,
			"message":      strings.TrimSpace(err.Error()),
			"provider":     constants.PaymentProviderEpay,
		})
		c.String(200, constants.EpayCallbackFail)
		return true
	}
	log.Infow("epay_callback_processed",
		"payment_id", payment.ID,
		"channel_id", channel.ID,
		"out_trade_no", outTradeNo,
		"status", updated.Status,
	)
	c.String(200, constants.EpayCallbackSuccess)
	return true
}
