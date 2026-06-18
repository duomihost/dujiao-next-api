package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/dujiaopay"
)

// dujiaoPayAdapter 是 DujiaoPay 的 Provider + Webhooker 实现。
// DujiaoPay 的 channel_type 使用文档里的 token_id，例如 tron-usdt / base-usdc。
type dujiaoPayAdapter struct{}

// NewDujiaoPayAdapter 实例化 DujiaoPay adapter。
func NewDujiaoPayAdapter() Provider { return &dujiaoPayAdapter{} }

var (
	_ Provider  = (*dujiaoPayAdapter)(nil)
	_ Webhooker = (*dujiaoPayAdapter)(nil)
)

// Type 返回 provider 标识。DujiaoPay 支持多个 token_id，因此 channelType 部分为空。
func (a *dujiaoPayAdapter) Type() string {
	return constants.PaymentProviderDujiaoPay + ":"
}

func (a *dujiaoPayAdapter) parseConfig(raw models.JSON, channelType string) (*dujiaopay.Config, error) {
	cfg, err := dujiaopay.ParseConfig(raw)
	if err != nil {
		return nil, mapDujiaoPayError(err)
	}
	channelType = strings.ToLower(strings.TrimSpace(channelType))
	if cfg.TokenID == "" && channelType != "" {
		cfg.TokenID = channelType
	}
	if cfg.Chain == "" && cfg.TokenID != "" {
		cfg.Chain = dujiaopay.ResolveChain(cfg.TokenID)
	}
	if err := dujiaopay.ValidateConfig(cfg); err != nil {
		return nil, mapDujiaoPayError(err)
	}
	return cfg, nil
}

// ValidateConfig 验证 DujiaoPay channel.ConfigJSON。
func (a *dujiaoPayAdapter) ValidateConfig(raw models.JSON, channelType string) error {
	channelType = strings.ToLower(strings.TrimSpace(channelType))
	if channelType != "" && !dujiaopay.IsSupportedTokenID(channelType) {
		return fmt.Errorf("%w: dujiaopay token_id %s", ErrUnsupportedChannel, channelType)
	}
	_, err := a.parseConfig(raw, channelType)
	return err
}

// CreatePayment 创建 DujiaoPay 收银台订单。
func (a *dujiaoPayAdapter) CreatePayment(ctx context.Context, raw models.JSON, input CreateInput) (*CreateResult, error) {
	channelType := strings.ToLower(strings.TrimSpace(input.ChannelType))
	if channelType != "" && !dujiaopay.IsSupportedTokenID(channelType) {
		return nil, fmt.Errorf("%w: dujiaopay token_id %s", ErrUnsupportedChannel, channelType)
	}

	cfg, err := a.parseConfig(raw, channelType)
	if err != nil {
		return nil, err
	}
	if rawFiatCurrency(raw) == "" && strings.TrimSpace(input.Currency) != "" {
		cfg.FiatCurrency = strings.ToUpper(strings.TrimSpace(input.Currency))
	}

	successURL := pickFirstNonEmpty(input.ReturnURL, cfg.SuccessURL)
	successURL = appendQueryParams(successURL, input.ReturnURLQuery)
	cancelURL := cfg.CancelURL
	if rawCancelURL, ok := input.Extra["cancel_url"].(string); ok && strings.TrimSpace(rawCancelURL) != "" {
		cancelURL = strings.TrimSpace(rawCancelURL)
	}

	metadata := map[string]interface{}{}
	if input.PaymentID > 0 {
		metadata["payment_id"] = input.PaymentID
	}
	if input.OrderID > 0 {
		metadata["order_id"] = input.OrderID
	}
	if input.Subject != "" {
		metadata["subject"] = input.Subject
	}

	result, err := dujiaopay.CreatePayment(ctx, cfg, dujiaopay.CreateInput{
		MerchantOrderID: strings.TrimSpace(input.OrderNo),
		FiatAmount:      input.Amount.Decimal.String(),
		SuccessURL:      successURL,
		CancelURL:       cancelURL,
		Metadata:        metadata,
	})
	if err != nil {
		return nil, mapDujiaoPayError(err)
	}

	qrCodeURL := strings.TrimSpace(result.CheckoutURL)
	if mode, ok := input.Extra["interaction_mode"].(string); ok && strings.ToLower(strings.TrimSpace(mode)) == constants.PaymentInteractionQR {
		qrCodeURL = pickFirstNonEmpty(result.PayAddress, result.CheckoutURL)
	}

	payload := models.JSON{}
	for key, value := range result.Raw {
		payload[key] = value
	}
	setIfNotEmpty := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			payload[key] = strings.TrimSpace(value)
		}
	}
	setIfNotEmpty("order_id", result.OrderID)
	setIfNotEmpty("chain", result.Chain)
	setIfNotEmpty("token_id", result.TokenID)
	setIfNotEmpty("pay_address", result.PayAddress)
	setIfNotEmpty("payable_amount", result.PayableAmount)
	setIfNotEmpty("checkout_url", result.CheckoutURL)

	return &CreateResult{
		ProviderRef: result.OrderID,
		RedirectURL: result.CheckoutURL,
		QRCodeURL:   qrCodeURL,
		Payload:     payload,
	}, nil
}

// ParseWebhook 验签并解析 DujiaoPay webhook。
func (a *dujiaoPayAdapter) ParseWebhook(_ context.Context, raw models.JSON, headers map[string]string, body []byte, now time.Time) (*WebhookResult, error) {
	cfg, err := dujiaopay.ParseConfig(raw)
	if err != nil {
		return nil, mapDujiaoPayError(err)
	}
	event, err := dujiaopay.ParseWebhook(cfg, headers, body, now)
	if err != nil {
		return nil, mapDujiaoPayError(err)
	}

	payload := models.JSON{}
	for key, value := range event.Raw {
		payload[key] = value
	}
	if event.TxHash != "" {
		payload["tx_hash"] = event.TxHash
	}

	return &WebhookResult{
		OrderNo:     event.MerchantOrderID,
		ProviderRef: event.OrderID,
		Status:      event.Status,
		PaidAt:      event.PaidAt,
		Payload:     payload,
	}, nil
}

func rawFiatCurrency(raw models.JSON) string {
	if raw == nil {
		return ""
	}
	if value, ok := raw["fiat_currency"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func mapDujiaoPayError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, dujiaopay.ErrUnsupportedToken):
		return fmt.Errorf("%w: %v", ErrUnsupportedChannel, err)
	case errors.Is(err, dujiaopay.ErrConfigInvalid):
		return fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	case errors.Is(err, dujiaopay.ErrRequestFailed):
		return fmt.Errorf("%w: %v", ErrRequestFailed, err)
	case errors.Is(err, dujiaopay.ErrResponseInvalid):
		return fmt.Errorf("%w: %v", ErrResponseInvalid, err)
	case errors.Is(err, dujiaopay.ErrSignatureInvalid):
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	default:
		return err
	}
}
