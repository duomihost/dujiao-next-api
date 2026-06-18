package public

import (
	"fmt"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *Handler) HandleAlipayCallback(c *gin.Context) bool {
	log := shared.RequestLog(c)
	form, err := parseCallbackForm(c)
	if err != nil {
		log.Warnw("alipay_callback_form_parse_failed", "error", err)
		return false
	}
	if !isAlipayCallbackForm(form) {
		log.Debugw("alipay_callback_not_matched")
		return false
	}
	log.Infow("alipay_callback_received",
		"client_ip", c.ClientIP(),
		"out_trade_no", strings.TrimSpace(getFirstValue(form, "out_trade_no")),
		"trade_no", strings.TrimSpace(getFirstValue(form, "trade_no")),
		"trade_status", strings.TrimSpace(getFirstValue(form, "trade_status")),
		"raw_form", callbackRawFormForLog(form),
	)

	payment, channel, err := h.findAlipayCallbackPayment(form)
	if err != nil || payment == nil || channel == nil {
		log.Warnw("alipay_callback_payment_not_found",
			"out_trade_no", strings.TrimSpace(getFirstValue(form, "out_trade_no")),
			"trade_no", strings.TrimSpace(getFirstValue(form, "trade_no")),
			"error", err,
		)
		c.String(200, constants.AlipayCallbackFail)
		return true
	}

	updated, err := h.PaymentService.HandleSyncCallback(channel, form, nil)
	if err != nil {
		log.Warnw("alipay_callback_handle_failed",
			"payment_id", payment.ID,
			"channel_id", channel.ID,
			"out_trade_no", strings.TrimSpace(getFirstValue(form, "out_trade_no")),
			"trade_no", strings.TrimSpace(getFirstValue(form, "trade_no")),
			"error", err,
		)
		h.enqueuePaymentExceptionAlert(c, models.JSON{
			"alert_type":   "alipay_callback_handle_failed",
			"alert_level":  "error",
			"payment_id":   fmt.Sprintf("%d", payment.ID),
			"out_trade_no": strings.TrimSpace(getFirstValue(form, "out_trade_no")),
			"message":      strings.TrimSpace(err.Error()),
			"provider":     constants.PaymentChannelTypeAlipay,
		})
		c.String(200, constants.AlipayCallbackFail)
		return true
	}
	log.Infow("alipay_callback_processed",
		"payment_id", payment.ID,
		"channel_id", channel.ID,
		"out_trade_no", strings.TrimSpace(getFirstValue(form, "out_trade_no")),
		"trade_no", strings.TrimSpace(getFirstValue(form, "trade_no")),
		"status", updated.Status,
	)
	c.String(200, constants.AlipayCallbackSuccess)
	return true
}

func isAlipayCallbackForm(form map[string][]string) bool {
	if strings.TrimSpace(getFirstValue(form, "sign")) == "" {
		return false
	}
	hasNotifyField := strings.TrimSpace(getFirstValue(form, "notify_id")) != "" ||
		strings.TrimSpace(getFirstValue(form, "notify_type")) != "" ||
		strings.TrimSpace(getFirstValue(form, "buyer_id")) != ""
	if !hasNotifyField {
		return false
	}
	if strings.TrimSpace(getFirstValue(form, "out_trade_no")) == "" && strings.TrimSpace(getFirstValue(form, "trade_no")) == "" {
		return false
	}
	return true
}

func (h *Handler) findAlipayCallbackPayment(form map[string][]string) (*models.Payment, *models.PaymentChannel, error) {
	outTradeNo := strings.TrimSpace(getFirstValue(form, "out_trade_no"))
	if outTradeNo != "" {
		payment, err := h.PaymentRepo.GetByGatewayOrderNo(outTradeNo)
		if err == nil && payment != nil {
			channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
			if err == nil && channel != nil &&
				strings.ToLower(strings.TrimSpace(channel.ProviderType)) == constants.PaymentProviderOfficial &&
				strings.ToLower(strings.TrimSpace(channel.ChannelType)) == constants.PaymentChannelTypeAlipay {
				return payment, channel, nil
			}
		}
	}

	tradeNo := strings.TrimSpace(getFirstValue(form, "trade_no"))
	if tradeNo != "" {
		payment, err := h.PaymentRepo.GetLatestByProviderRef(tradeNo)
		if err == nil && payment != nil {
			channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
			if err == nil && channel != nil &&
				strings.ToLower(strings.TrimSpace(channel.ProviderType)) == constants.PaymentProviderOfficial &&
				strings.ToLower(strings.TrimSpace(channel.ChannelType)) == constants.PaymentChannelTypeAlipay {
				return payment, channel, nil
			}
		}
	}
	return nil, nil, service.ErrPaymentNotFound
}
