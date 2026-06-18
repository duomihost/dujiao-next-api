package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/epay"

	"github.com/shopspring/decimal"
)

// epayAdapter 是 epay 网关的 Provider + CallbackVerifier 实现。
// epay 没有主动查询 API，callback 是同步 form POST（不是 JSON webhook），
// 所以**不**实现 Capturer 和 Webhooker。
type epayAdapter struct{}

// NewEpayAdapter 实例化 epay adapter。
func NewEpayAdapter() Provider { return &epayAdapter{} }

// 编译期断言 epayAdapter 实现了 Provider 和 CallbackVerifier。
var (
	_ Provider         = (*epayAdapter)(nil)
	_ CallbackVerifier = (*epayAdapter)(nil)
)

// Type 返回 provider 标识。
func (a *epayAdapter) Type() string {
	return constants.PaymentProviderEpay + ":"
}

// parseConfig 解析并验证 epay Config。epay 不需要 interactionMode，
// 通过 dispatch 时按 mode 调用不同的函数（BuildRedirectURL vs CreatePayment）。
func (a *epayAdapter) parseConfig(raw models.JSON) (*epay.Config, error) {
	cfg, err := epay.ParseConfig(raw)
	if err != nil {
		return nil, mapEpayError(err)
	}
	if err := epay.ValidateConfig(cfg); err != nil {
		return nil, mapEpayError(err)
	}
	return cfg, nil
}

// ValidateConfig 验证 channel.ConfigJSON。
// 入口先校验 channelType（如果非空）是否被 epay 支持，
// 然后调用 parseConfig 验证配置完整性。
func (a *epayAdapter) ValidateConfig(raw models.JSON, channelType string) error {
	if channelType != "" && !epay.IsSupportedChannelType(channelType) {
		return fmt.Errorf("%w: epay channel_type %s", ErrUnsupportedChannel, channelType)
	}
	_, err := a.parseConfig(raw)
	return err
}

// CreatePayment 创建支付。双 mode dispatch：
//   - mode == "redirect" → BuildRedirectURL（不发 HTTP，仅构造跳转 URL）
//   - mode == "" 或 mode == "qr" → CreatePayment（发 HTTP）
//   - 其他 mode → 返回 ErrConfigInvalid
func (a *epayAdapter) CreatePayment(ctx context.Context, raw models.JSON, input CreateInput) (*CreateResult, error) {
	// 先校验 channelType
	if !epay.IsSupportedChannelType(input.ChannelType) {
		return nil, fmt.Errorf("%w: epay channel_type %s", ErrUnsupportedChannel, input.ChannelType)
	}

	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}

	// NotifyURL / ReturnURL: 优先用 input 传入值，为空则 fallback 到 cfg 里的值。
	// epay native 硬校验这两个字段非空，所以必须做 fallback，否则 caller 若没传就会报错。
	notifyURL := strings.TrimSpace(input.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(cfg.NotifyURL)
	}
	returnURL := strings.TrimSpace(input.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(cfg.ReturnURL)
	}
	// P1.2c Task 3: fallback 完成后 append tracking marker。
	returnURL = appendQueryParams(returnURL, input.ReturnURLQuery)

	// P1.2c: wrapper 内做 currency conversion + audit 字段写入，在 mode dispatch 之前完成，
	// 使两条 path（BuildRedirectURL / CreatePayment）都使用转换后金额。
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

	native := epay.CreateInput{
		OrderNo:     input.OrderNo,
		Amount:      payAmount,
		Subject:     input.Subject,
		ChannelType: input.ChannelType,
		ClientIP:    input.ClientIP,
		NotifyURL:   notifyURL,
		ReturnURL:   returnURL,
	}

	// 从 input.Extra 取 interaction_mode，按模式 dispatch
	mode, _ := input.Extra["interaction_mode"].(string)
	mode = strings.ToLower(strings.TrimSpace(mode))

	var result *epay.CreateResult
	switch mode {
	case constants.PaymentInteractionRedirect:
		result, err = epay.BuildRedirectURL(cfg, native)
	case "", constants.PaymentInteractionQR:
		result, err = epay.CreatePayment(ctx, cfg, native)
	default:
		return nil, fmt.Errorf("%w: epay interaction_mode %s", ErrConfigInvalid, mode)
	}

	if err != nil {
		return nil, mapEpayError(err)
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
		ProviderRef:  result.TradeNo,
		RedirectURL:  result.PayURL,
		QRCodeURL:    result.QRCode,
		Payload:      payload,
		AmountSent:   payAmount,
		CurrencySent: payCurrency,
	}, nil
}

// VerifyCallback 实现 CallbackVerifier。epay 用 form POST，body 参数忽略。
// epay 同步 callback form 字段：
//   - out_trade_no = 商户单号(OrderNo)
//   - trade_no     = epay 网关单号(ProviderRef)
//   - trade_status = TRADE_SUCCESS / TRADE_FINISHED 等
//   - money        = 金额
//   - sign / sign_type / type / pid / param 等元数据
func (a *epayAdapter) VerifyCallback(raw models.JSON, form map[string][]string, _ []byte) (*CallbackResult, error) {
	cfg, err := epay.ParseConfig(raw)
	if err != nil {
		return nil, mapEpayError(err)
	}

	if err := epay.VerifyCallback(cfg, form); err != nil {
		return nil, mapEpayError(err)
	}
	if err := epay.VerifyCallbackOwnership(cfg, form); err != nil {
		return nil, mapEpayError(err)
	}

	orderNo := pickFormValue(form, "out_trade_no")
	providerRef := pickFormValue(form, "trade_no")
	tradeStatus := pickFormValue(form, "trade_status")
	amountStr := pickFormValue(form, "money")

	status := constants.PaymentStatusPending
	if tradeStatus == "TRADE_SUCCESS" || tradeStatus == "TRADE_FINISHED" {
		status = constants.PaymentStatusSuccess
	}

	// amount 解析失败时返回零值：wrapper 仅做适配，金额异常由业务层判定。
	amount := models.Money{}
	if s := strings.TrimSpace(amountStr); s != "" {
		if d, parseErr := decimal.NewFromString(s); parseErr == nil {
			amount = models.NewMoneyFromDecimal(d)
		}
	}

	return &CallbackResult{
		OrderNo:     orderNo,
		ProviderRef: providerRef,
		Status:      status,
		Amount:      amount,
		Currency:    "CNY",
		PaidAt:      nil, // epay callback 不带付款时间字段
		Payload:     formToJSON(form),
	}, nil
}

// mapEpayError 把 epay 包的 sentinel error 映射为 provider 统一错误。
func mapEpayError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, epay.ErrConfigInvalid):
		return fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	case errors.Is(err, epay.ErrChannelTypeNotOK):
		// P1.2a Task 1 加的 ErrUnsupportedChannel 就是给这个用的
		return fmt.Errorf("%w: %v", ErrUnsupportedChannel, err)
	case errors.Is(err, epay.ErrSignatureGenerate):
		// 签名生成失败 = 配置层面错误(私钥不可用)
		return fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	case errors.Is(err, epay.ErrSignatureInvalid):
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	case errors.Is(err, epay.ErrRequestFailed):
		return fmt.Errorf("%w: %v", ErrRequestFailed, err)
	case errors.Is(err, epay.ErrResponseInvalid):
		return fmt.Errorf("%w: %v", ErrResponseInvalid, err)
	default:
		return err
	}
}
