package public

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/models"

	"github.com/gin-gonic/gin"
)

func (h *Handler) HandleOkpayCallback(c *gin.Context) bool {
	log := shared.RequestLog(c)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// 轻量级特征检测：从 form 或 JSON body 中探测 sign + data[unique_id] + data[order_id]
	// okpay callback 是 form POST（application/x-www-form-urlencoded）
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		log.Debugw("okpay_callback_not_matched", "reason", "empty_body")
		return false
	}
	probeForm, parseErr := url.ParseQuery(trimmed)
	if parseErr != nil {
		log.Debugw("okpay_callback_parse_failed", "error", parseErr)
		return false
	}
	sign := strings.TrimSpace(probeForm.Get("sign"))
	uniqueID := strings.TrimSpace(probeForm.Get("data[unique_id]"))
	orderID := strings.TrimSpace(probeForm.Get("data[order_id]"))
	if sign == "" || (uniqueID == "" && orderID == "") {
		log.Debugw("okpay_callback_not_matched", "reason", "missing_sign_or_ids")
		return false
	}

	log.Infow("okpay_callback_received",
		"unique_id", uniqueID,
		"order_id", orderID,
		"raw_body", callbackRawBodyForLog(body),
	)

	payment, err := h.PaymentRepo.GetByGatewayOrderNo(uniqueID)
	if err != nil || payment == nil {
		payment, err = h.PaymentRepo.GetLatestByProviderRef(orderID)
		if err != nil || payment == nil {
			log.Warnw("okpay_callback_payment_not_found", "unique_id", uniqueID, "order_id", orderID, "error", err)
			c.Data(200, "application/json", []byte(constants.OkpayCallbackFail))
			return true
		}
	}

	log.Debugw("okpay_callback_payment_found", "payment_id", payment.ID, "channel_id", payment.ChannelID)

	channel, err := h.PaymentChannelRepo.GetByID(payment.ChannelID)
	if err != nil || channel == nil {
		log.Warnw("okpay_callback_channel_not_found", "payment_id", payment.ID, "channel_id", payment.ChannelID, "error", err)
		c.Data(200, "application/json", []byte(constants.OkpayCallbackFail))
		return true
	}
	if strings.ToLower(strings.TrimSpace(channel.ProviderType)) != constants.PaymentProviderOkpay {
		log.Warnw("okpay_callback_provider_invalid", "provider_type", channel.ProviderType)
		c.Data(200, "application/json", []byte(constants.OkpayCallbackFail))
		return true
	}

	updated, err := h.PaymentService.HandleSyncCallback(channel, nil, body)
	if err != nil {
		log.Errorw("okpay_callback_handle_failed", "payment_id", payment.ID, "error", err)
		h.enqueuePaymentExceptionAlert(c, models.JSON{
			"alert_type":  "okpay_callback_handle_failed",
			"alert_level": "error",
			"payment_id":  fmt.Sprintf("%d", payment.ID),
			"unique_id":   uniqueID,
			"message":     strings.TrimSpace(err.Error()),
			"provider":    constants.PaymentProviderOkpay,
		})
		c.Data(200, "application/json", []byte(constants.OkpayCallbackFail))
		return true
	}

	log.Infow("okpay_callback_processed", "payment_id", payment.ID, "status", updated.Status)
	c.Data(200, "application/json", []byte(constants.OkpayCallbackSuccess))
	return true
}
