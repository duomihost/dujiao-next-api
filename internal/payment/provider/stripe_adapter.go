package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/stripe"

	"github.com/shopspring/decimal"
)

// stripeAdapter 是 stripe 网关的 Provider/Capturer/Webhooker 实现。
// 它仅做参数适配 + 错误映射，所有业务逻辑仍委托给 internal/payment/stripe/ 包级函数。
type stripeAdapter struct{}

// NewStripeAdapter 实例化 stripe adapter。
func NewStripeAdapter() Provider { return &stripeAdapter{} }

// 编译期断言 stripeAdapter 实现了三个 capability interface。
var (
	_ Provider  = (*stripeAdapter)(nil)
	_ Capturer  = (*stripeAdapter)(nil)
	_ Webhooker = (*stripeAdapter)(nil)
)

// Type 返回 provider 标识。
func (a *stripeAdapter) Type() string {
	return constants.PaymentProviderOfficial + ":" + constants.PaymentChannelTypeStripe
}

// parseConfig 解析并验证 stripe Config，把 stripe.ErrConfigInvalid 等映射为 provider.ErrXxx。
// 4 个公开方法共用，避免每个都重复 6 行样板。
func (a *stripeAdapter) parseConfig(raw models.JSON) (*stripe.Config, error) {
	cfg, err := stripe.ParseConfig(raw)
	if err != nil {
		return nil, mapStripeError(err)
	}
	if err := stripe.ValidateConfig(cfg); err != nil {
		return nil, mapStripeError(err)
	}
	return cfg, nil
}

// ValidateConfig 验证 channel.ConfigJSON。
// 第二参数 interactionMode 由 admin 端 ValidateChannel 传入；stripe 只支持 redirect 模式。
// 若传空字符串（非 admin 端调用），不做 interactionMode 校验，以保持向后兼容。
func (a *stripeAdapter) ValidateConfig(raw models.JSON, interactionMode string) error {
	if interactionMode != "" && strings.ToLower(strings.TrimSpace(interactionMode)) != constants.PaymentInteractionRedirect {
		return fmt.Errorf("%w: stripe only supports redirect interaction_mode", ErrConfigInvalid)
	}
	_, err := a.parseConfig(raw)
	return err
}

// CreatePayment 创建支付。
func (a *stripeAdapter) CreatePayment(ctx context.Context, raw models.JSON, input CreateInput) (*CreateResult, error) {
	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}

	// P1.2c: wrapper 内做 currency conversion + audit 字段写入。
	// exchange_rate / original_amount / original_currency 保留到 result.Payload，
	// 供运营/财务跨币种对账追溯实际收费 vs 原始金额。
	// result.AmountSent/CurrencySent 反映实际发给网关的金额/币种，
	// 让 service 层据此更新 payment.Amount/Currency，保持记录与实际收费一致。
	originalAmount := input.Amount.Decimal.String()
	originalCurrency := input.Currency
	payAmount := originalAmount
	payCurrency := originalCurrency
	converted := false
	if cfg.NeedsCurrencyConversion() {
		convAmount, convCurrency, convErr := cfg.ConvertAmount(payAmount, payCurrency, 2)
		if convErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrConfigInvalid, convErr)
		}
		payAmount = convAmount
		payCurrency = convCurrency
		converted = true
	}

	// P1.2c Task 3: 先 fallback 到 cfg.SuccessURL，再 append tracking marker。
	// stripe 用 SuccessURL 对应 ReturnURL。CancelURL 不 append marker（取消路径无需识别 biz_type）。
	successURL := strings.TrimSpace(input.ReturnURL)
	if successURL == "" {
		successURL = strings.TrimSpace(cfg.SuccessURL)
	}
	successURL = appendQueryParams(successURL, input.ReturnURLQuery)

	cancelURL, _ := input.Extra["cancel_url"].(string)
	native := stripe.CreateInput{
		OrderNo:     input.OrderNo,
		Amount:      payAmount,
		Currency:    payCurrency,
		Description: input.Subject,
		SuccessURL:  successURL,
		CancelURL:   cancelURL,
	}
	result, err := stripe.CreatePayment(ctx, cfg, native)
	if err != nil {
		return nil, mapStripeError(err)
	}

	payload := models.JSON{}
	if result.Raw != nil {
		payload = models.JSON(result.Raw)
	}
	if converted {
		payload["exchange_rate"] = strings.TrimSpace(cfg.ExchangeRate)
		payload["original_amount"] = originalAmount
		payload["original_currency"] = originalCurrency
	}

	return &CreateResult{
		ProviderRef:  pickFirstNonEmpty(result.SessionID, result.PaymentIntentID),
		RedirectURL:  result.URL,
		Payload:      payload,
		AmountSent:   payAmount,
		CurrencySent: payCurrency,
	}, nil
}

// QueryPayment 主动查询订单状态(实现 Capturer)。
func (a *stripeAdapter) QueryPayment(ctx context.Context, raw models.JSON, providerRef string) (*QueryResult, error) {
	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}

	result, err := stripe.QueryPayment(ctx, cfg, providerRef)
	if err != nil {
		return nil, mapStripeError(err)
	}

	// amount 解析失败时返回零值：wrapper 仅做适配，金额异常的语义边界(对账失败 / 网关返回脏数据)
	// 留给上游业务层判定，wrapper 不擅自报错。
	amount := models.Money{}
	if s := strings.TrimSpace(result.Amount); s != "" {
		if parsed, parseErr := decimal.NewFromString(s); parseErr == nil {
			amount = models.NewMoneyFromDecimal(parsed)
		}
	}

	return &QueryResult{
		ProviderRef: pickFirstNonEmpty(result.SessionID, result.PaymentIntentID, providerRef),
		Status:      result.Status,
		Amount:      amount,
		Currency:    strings.ToUpper(strings.TrimSpace(result.Currency)),
		PaidAt:      result.PaidAt,
		Payload:     models.JSON(result.Raw),
	}, nil
}

// ParseWebhook 验签并解析 webhook(实现 Webhooker)。
func (a *stripeAdapter) ParseWebhook(_ context.Context, raw models.JSON, headers map[string]string, body []byte, now time.Time) (*WebhookResult, error) {
	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}

	result, err := stripe.VerifyAndParseWebhook(cfg, headers, body, now)
	if err != nil {
		return nil, mapStripeError(err)
	}

	// amount 解析失败时返回零值：wrapper 仅做适配，金额异常的语义边界(对账失败 / 网关返回脏数据)
	// 留给上游业务层判定，wrapper 不擅自报错。
	amount := models.Money{}
	if s := strings.TrimSpace(result.Amount); s != "" {
		if parsed, parseErr := decimal.NewFromString(s); parseErr == nil {
			amount = models.NewMoneyFromDecimal(parsed)
		}
	}

	return &WebhookResult{
		OrderNo:     result.OrderNo,
		ProviderRef: pickFirstNonEmpty(result.ProviderRef, result.SessionID, result.PaymentIntentID),
		Status:      result.Status,
		Amount:      amount,
		Currency:    strings.ToUpper(strings.TrimSpace(result.Currency)),
		PaidAt:      result.PaidAt,
		Payload:     models.JSON(result.Raw),
	}, nil
}

func mapStripeError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, stripe.ErrConfigInvalid):
		return fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	case errors.Is(err, stripe.ErrRequestFailed):
		return fmt.Errorf("%w: %v", ErrRequestFailed, err)
	case errors.Is(err, stripe.ErrResponseInvalid):
		return fmt.Errorf("%w: %v", ErrResponseInvalid, err)
	case errors.Is(err, stripe.ErrSignatureInvalid):
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	default:
		return err
	}
}

// pickFirstNonEmpty 返回第一个非空字符串。
func pickFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
