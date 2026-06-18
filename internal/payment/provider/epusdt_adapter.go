package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/epusdt"

	"github.com/shopspring/decimal"
)

// epusdtAdapter 是 epusdt 网关的 Provider + CallbackVerifier 实现。
// epusdt 没有主动查询 API，callback 是同步 JSON POST（不是 form），
// 所以**不**实现 Capturer 和 Webhooker。
type epusdtAdapter struct{}

// NewEpusdtAdapter 实例化 epusdt adapter。
func NewEpusdtAdapter() Provider { return &epusdtAdapter{} }

// 编译期断言 epusdtAdapter 实现了 Provider 和 CallbackVerifier。
var (
	_ Provider         = (*epusdtAdapter)(nil)
	_ CallbackVerifier = (*epusdtAdapter)(nil)
)

// Type 返回 provider 标识。epusdt 是单 channel type provider，返回值中 channelType 部分为空。
func (a *epusdtAdapter) Type() string {
	return constants.PaymentProviderEpusdt + ":"
}

// parseConfig 解析并验证 epusdt Config。epusdt 不需要 interactionMode。
func (a *epusdtAdapter) parseConfig(raw models.JSON) (*epusdt.Config, error) {
	cfg, err := epusdt.ParseConfig(raw)
	if err != nil {
		return nil, mapEpusdtError(err)
	}
	if err := epusdt.ValidateConfig(cfg); err != nil {
		return nil, mapEpusdtError(err)
	}
	return cfg, nil
}

// ValidateConfig 验证 channel.ConfigJSON。
func (a *epusdtAdapter) ValidateConfig(raw models.JSON, _ string) error {
	_, err := a.parseConfig(raw)
	return err
}

// CreatePayment 创建支付。epusdt 单 channel type，不需要 IsSupportedChannelType 校验。
func (a *epusdtAdapter) CreatePayment(ctx context.Context, raw models.JSON, input CreateInput) (*CreateResult, error) {
	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}

	returnURL := strings.TrimSpace(input.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(cfg.ReturnURL)
	}
	returnURL = appendQueryParams(returnURL, input.ReturnURLQuery)

	native := epusdt.CreateInput{
		OrderNo:   input.OrderNo,
		Amount:    input.Amount.Decimal.String(),
		Name:      input.Subject,
		NotifyURL: input.NotifyURL,
		ReturnURL: returnURL,
	}
	result, err := epusdt.CreatePayment(ctx, cfg, native)
	if err != nil {
		return nil, mapEpusdtError(err)
	}

	return &CreateResult{
		ProviderRef: result.TradeID,
		RedirectURL: result.PaymentURL,
		QRCodeURL:   result.PaymentURL, // epusdt 是 USDT 网关，PaymentURL 同时用于跳转和 QR 展示
		Payload:     models.JSON(result.Raw),
	}, nil
}

// VerifyCallback 实现 CallbackVerifier。epusdt 用 JSON POST body，form 参数忽略。
func (a *epusdtAdapter) VerifyCallback(raw models.JSON, _ map[string][]string, body []byte) (*CallbackResult, error) {
	cfg, err := epusdt.ParseConfig(raw)
	if err != nil {
		return nil, mapEpusdtError(err)
	}

	data, err := epusdt.ParseCallback(body)
	if err != nil {
		return nil, mapEpusdtError(err)
	}

	if err := epusdt.VerifyCallback(cfg, data); err != nil {
		return nil, mapEpusdtError(err)
	}

	// epusdt 用 status int → PaymentStatusXxx string 映射
	status := epusdt.ToPaymentStatus(data.Status)

	// amount 解析失败时返回零值：wrapper 仅做适配，金额异常由业务层判定。
	amount := models.Money{}
	if data.Amount != nil {
		amountFloat := data.GetAmount()
		if amountFloat > 0 {
			amount = models.NewMoneyFromDecimal(decimal.NewFromFloat(amountFloat))
		}
	}

	// 把 callback 关键字段塞进 Payload
	payload := models.JSON{}
	if pb, marshalErr := json.Marshal(data); marshalErr == nil {
		var m map[string]interface{}
		if jsonErr := json.Unmarshal(pb, &m); jsonErr == nil {
			payload = models.JSON(m)
		}
	}

	return &CallbackResult{
		OrderNo:     data.OrderID,
		ProviderRef: data.TradeID,
		Status:      status,
		Amount:      amount,
		Currency:    strings.ToUpper(cfg.Currency),
		PaidAt:      nil, // epusdt callback 不带付款时间
		Payload:     payload,
	}, nil
}

// mapEpusdtError 把 epusdt 包的 sentinel error 映射为 provider 统一错误。
func mapEpusdtError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, epusdt.ErrConfigInvalid):
		return fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	case errors.Is(err, epusdt.ErrRequestFailed):
		return fmt.Errorf("%w: %v", ErrRequestFailed, err)
	case errors.Is(err, epusdt.ErrResponseInvalid):
		return fmt.Errorf("%w: %v", ErrResponseInvalid, err)
	case errors.Is(err, epusdt.ErrSignatureInvalid):
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	default:
		return err
	}
}
