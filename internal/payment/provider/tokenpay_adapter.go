package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/tokenpay"

	"github.com/shopspring/decimal"
)

// tokenpayAdapter 是 tokenpay 网关的 Provider + CallbackVerifier 实现。
// tokenpay 采用 JSON callback 模式（同步 POST），不支持 Capturer 和 Webhooker。
type tokenpayAdapter struct{}

// NewTokenpayAdapter 实例化 tokenpay adapter。
func NewTokenpayAdapter() Provider { return &tokenpayAdapter{} }

// 编译期断言 tokenpayAdapter 实现了 Provider 和 CallbackVerifier。
var (
	_ Provider         = (*tokenpayAdapter)(nil)
	_ CallbackVerifier = (*tokenpayAdapter)(nil)
)

// Type 返回 provider 标识。tokenpay 是单 channel type provider，返回值中 channelType 部分为空。
func (a *tokenpayAdapter) Type() string {
	return constants.PaymentProviderTokenpay + ":"
}

// parseConfig 解析并验证 tokenpay Config。tokenpay 不需要 interactionMode。
func (a *tokenpayAdapter) parseConfig(raw models.JSON) (*tokenpay.Config, error) {
	cfg, err := tokenpay.ParseConfig(raw)
	if err != nil {
		return nil, mapTokenpayError(err)
	}
	cfg.Normalize()
	if err := tokenpay.ValidateConfig(cfg); err != nil {
		return nil, mapTokenpayError(err)
	}
	return cfg, nil
}

// ValidateConfig 验证 channel.ConfigJSON。
func (a *tokenpayAdapter) ValidateConfig(raw models.JSON, _ string) error {
	_, err := a.parseConfig(raw)
	return err
}

// CreatePayment 创建支付。tokenpay 单 channel type，不需要 IsSupportedChannelType 校验。
func (a *tokenpayAdapter) CreatePayment(ctx context.Context, raw models.JSON, input CreateInput) (*CreateResult, error) {
	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}

	// OrderUserKey 必填，从 input.Extra["order_user_key"] 取
	// tokenpay 特殊字段，用户标识符
	orderUserKey, _ := input.Extra["order_user_key"].(string)

	redirectURL := strings.TrimSpace(input.ReturnURL)
	if redirectURL == "" {
		redirectURL = strings.TrimSpace(cfg.RedirectURL)
	}
	redirectURL = appendQueryParams(redirectURL, input.ReturnURLQuery)

	// Currency 是 TokenPay 的“加密货币币种”（如 USDT_TRC20 / TRX），必须取自渠道配置 cfg.Currency。
	// 切勿使用 input.Currency——那是订单法币币种（CNY/USD 等），它对应 TokenPay 的 ActualAmount 金额币种。
	native := tokenpay.CreateInput{
		OutOrderID:   input.OrderNo,
		OrderUserKey: orderUserKey,
		ActualAmount: input.Amount.Decimal.String(),
		Currency:     cfg.Currency,
		NotifyURL:    input.NotifyURL,
		RedirectURL:  redirectURL,
	}
	result, err := tokenpay.CreatePayment(ctx, cfg, native)
	if err != nil {
		return nil, mapTokenpayError(err)
	}

	// QRCodeLink 优先，QRCodeBase64 备选
	qrCode := result.QRCodeLink
	if qrCode == "" {
		qrCode = result.QRCodeBase64
	}

	return &CreateResult{
		ProviderRef: result.TokenOrderID,
		RedirectURL: result.PayURL,
		QRCodeURL:   qrCode,
		Payload:     models.JSON(result.Raw),
	}, nil
}

// VerifyCallback 实现 CallbackVerifier。tokenpay 用 JSON POST body，form 参数忽略。
// 注意：tokenpay.VerifyCallback 签名特殊，第一参数 data，第二参数 notifySecret string。
func (a *tokenpayAdapter) VerifyCallback(raw models.JSON, _ map[string][]string, body []byte) (*CallbackResult, error) {
	cfg, err := tokenpay.ParseConfig(raw)
	if err != nil {
		return nil, mapTokenpayError(err)
	}

	data, err := tokenpay.ParseCallback(body)
	if err != nil {
		return nil, mapTokenpayError(err)
	}

	// tokenpay.VerifyCallback 签名特殊：第一参数 data，第二参数 cfg.NotifySecret string
	if err := tokenpay.VerifyCallback(data, cfg.NotifySecret); err != nil {
		return nil, mapTokenpayError(err)
	}

	// tokenpay 用 status int → PaymentStatusXxx string 映射
	status := tokenpay.ToPaymentStatus(data.Status)

	// 金额口径必须是法币（与订单 payment.Amount 一致）：TokenPay 回调里 ActualAmount 是法币金额，
	// 而 Amount 是法币换算后的加密货币数量。业务层会用回调金额与 payment.Amount 严格比对，故取 ActualAmount。
	// amount silent-fallback：失败时返回零值，wrapper 仅做适配，金额异常由业务层判定
	amount := models.Money{}
	if s := strings.TrimSpace(data.ActualAmount); s != "" {
		if d, parseErr := decimal.NewFromString(s); parseErr == nil {
			amount = models.NewMoneyFromDecimal(d)
		}
	}

	// PayTime 用 tokenpay.ParsePaidAt 解析（tokenpay 包暴露的 helper，处理时区）
	paidAt := tokenpay.ParsePaidAt(data.PayTime)

	// Currency 口径必须是法币（与订单 payment.Currency 一致）：TokenPay 回调里 BaseCurrency 是法币币种，
	// 而 Currency 是加密货币币种（USDT_TRC20/TRX 等）。优先用回调 BaseCurrency，fallback cfg.BaseCurrency。
	currency := strings.ToUpper(strings.TrimSpace(data.BaseCurrency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(cfg.BaseCurrency))
	}

	// Payload 通过 json.Marshal/Unmarshal CallbackData 序列化
	payload := models.JSON{}
	if pb, marshalErr := json.Marshal(data); marshalErr == nil {
		var m map[string]interface{}
		if jsonErr := json.Unmarshal(pb, &m); jsonErr == nil {
			payload = models.JSON(m)
		}
	}

	return &CallbackResult{
		OrderNo:     data.OutOrderID,
		ProviderRef: data.TokenOrderID,
		Status:      status,
		Amount:      amount,
		Currency:    currency,
		PaidAt:      paidAt,
		Payload:     payload,
	}, nil
}

// mapTokenpayError 把 tokenpay 包的 sentinel error 映射为 provider 统一错误。
func mapTokenpayError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, tokenpay.ErrConfigInvalid):
		return fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	case errors.Is(err, tokenpay.ErrRequestFailed):
		return fmt.Errorf("%w: %v", ErrRequestFailed, err)
	case errors.Is(err, tokenpay.ErrResponseInvalid):
		return fmt.Errorf("%w: %v", ErrResponseInvalid, err)
	case errors.Is(err, tokenpay.ErrSignatureInvalid):
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	default:
		return err
	}
}
