package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/provider"

	"github.com/shopspring/decimal"
)

func (s *PaymentService) applyProviderPayment(input CreatePaymentInput, order *models.Order, channel *models.PaymentChannel, payment *models.Payment) (err error) {
	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
	gatewayCtx, cancel := detachOutboundRequestContext(input.Context)
	defer cancel()
	payment.GatewayOrderNo = resolveGatewayOrderNo(channel, payment)
	providerOrderNo := resolveProviderOrderNo(order.OrderNo, payment)
	log := paymentLogger(
		"order_id", order.ID,
		"order_no", order.OrderNo,
		"gateway_order_no", payment.GatewayOrderNo,
		"payment_id", payment.ID,
		"channel_id", channel.ID,
		"provider_type", providerType,
		"channel_type", channelType,
		"interaction_mode", channel.InteractionMode,
	)
	defer func() {
		if err != nil {
			log.Errorw("payment_provider_apply_failed", "error", err)
			return
		}
		log.Infow("payment_provider_apply_success")
	}()
	if s.paymentProviderRegistry == nil {
		return ErrPaymentProviderNotSupported
	}

	p, ok := s.paymentProviderRegistry.Lookup(channel.ProviderType, channel.ChannelType)
	if !ok {
		return ErrPaymentProviderNotSupported
	}

	// 构造 provider.CreateInput。
	// NotifyURL / ReturnURL 留空：各 adapter/native 包均实现 "input值 || cfg值" fallback，
	// 空值时自动读取 channel.ConfigJSON 里配置的 notify_url / return_url。
	// P1.2c Task 3: returnURLQuery 携带 biz_type/order_no/marker 等，由 wrapper append 到 ReturnURL。
	extra := models.JSON{}
	if interactionMode := strings.TrimSpace(channel.InteractionMode); interactionMode != "" {
		extra["interaction_mode"] = interactionMode
	}
	// order_user_key 是 tokenpay 必须的稳定用户标识符；其他 adapter 忽略此字段。
	extra["order_user_key"] = resolveTokenPayOrderUserKey(order)

	// P1.2c Task 3: 构造 return URL tracking marker。
	// official provider 用 channelType 区分网关(paypal/alipay/wechat/stripe)，其他 provider 用 providerType。
	returnMarker := providerType + "_return"
	if providerType == constants.PaymentProviderOfficial {
		returnMarker = channelType + "_return"
	}
	returnURLQuery := buildPaymentReturnQuery(input, order, returnMarker, "")

	createInput := provider.CreateInput{
		PaymentID:      payment.ID,
		OrderID:        order.ID,
		OrderNo:        providerOrderNo,
		Subject:        buildOrderSubject(order),
		Amount:         payment.Amount,
		Currency:       payment.Currency,
		ClientIP:       strings.TrimSpace(input.ClientIP),
		ChannelType:    channel.ChannelType,
		Extra:          extra,
		ReturnURLQuery: returnURLQuery,
		// NotifyURL / ReturnURL 留空，由各 adapter 从 cfg 读取，再 append ReturnURLQuery
	}

	result, err := p.CreatePayment(gatewayCtx, channel.ConfigJSON, createInput)
	if err != nil {
		return mapProviderErrorToService(err)
	}

	// 把 result 写回 payment 字段
	payment.PayURL = strings.TrimSpace(result.RedirectURL)
	payment.QRCode = strings.TrimSpace(result.QRCodeURL)
	if result.ProviderRef != "" {
		payment.ProviderRef = result.ProviderRef
	}
	// 确保 ProviderRef 始终有值（各 adapter 可能返回空，如 wechat CreatePayment 阶段）。
	// 主动查询必须使用实际提交到网关的订单号，而不是业务订单号。
	if payment.ProviderRef == "" {
		payment.ProviderRef = providerOrderNo
	}
	if result.Payload != nil {
		payment.ProviderPayload = result.Payload
	}
	// P1.2c: 用 wrapper 转换后的 amount/currency 更新 payment 记录，
	// 保持 DB 状态与实际发给网关的金额/币种一致（P1.2c Task 1 把 conversion 下沉到 wrapper）。
	if strings.TrimSpace(result.CurrencySent) != "" {
		payment.Currency = result.CurrencySent
	}
	if strings.TrimSpace(result.AmountSent) != "" {
		if d, parseErr := decimal.NewFromString(result.AmountSent); parseErr == nil {
			payment.Amount = models.Money{Decimal: d}
		}
	}
	payment.Status = constants.PaymentStatusPending
	payment.UpdatedAt = time.Now()

	if err := s.paymentRepo.Update(payment); err != nil {
		return ErrPaymentUpdateFailed
	}
	return nil
}

// ValidateChannel 校验支付渠道配置（admin 端 channel 创建/更新时调用）。
//
// P1.2c Task 10: 160 行 switch 退化为 Registry.Lookup + Provider.ValidateConfig 单点。
// 基础字段校验（nil / fee / amount / providerType / wallet 跳过）保留在 service 层；
// 配置格式验证通过 Registry 委托给各 adapter wrapper。
func (s *PaymentService) ValidateChannel(channel *models.PaymentChannel) error {
	if channel == nil {
		return ErrPaymentChannelConfigInvalid
	}
	feeRate := channel.FeeRate.Decimal.Round(2)
	if feeRate.LessThan(decimal.Zero) || feeRate.GreaterThan(decimal.NewFromInt(100)) {
		return ErrPaymentChannelConfigInvalid
	}
	fixedFee := channel.FixedFee.Decimal.Round(2)
	// decimal(6,2) max value is 9999.99
	if fixedFee.LessThan(decimal.Zero) || fixedFee.GreaterThanOrEqual(decimal.NewFromInt(10000)) {
		return ErrPaymentChannelConfigInvalid
	}
	minAmount := channel.MinAmount.Decimal.Round(2)
	maxAmount := channel.MaxAmount.Decimal.Round(2)
	amountOverflow20_2 := decimal.NewFromInt(1000000000000000000)
	// min/max amount are stored as decimal(20,2), max allowed is 999999999999999999.99.
	if minAmount.LessThan(decimal.Zero) || minAmount.GreaterThanOrEqual(amountOverflow20_2) || maxAmount.LessThan(decimal.Zero) || maxAmount.GreaterThanOrEqual(amountOverflow20_2) {
		return ErrPaymentChannelConfigInvalid
	}
	if maxAmount.GreaterThan(decimal.Zero) && minAmount.GreaterThan(maxAmount) {
		return ErrPaymentChannelConfigInvalid
	}

	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	channelType := strings.ToLower(strings.TrimSpace(channel.ChannelType))
	if providerType == "" {
		return ErrPaymentChannelConfigInvalid
	}

	// wallet 是内部余额通道，无 native adapter，直接通过。
	if providerType == constants.PaymentProviderWallet {
		return nil
	}

	// 非 official provider（epay/bepusdt/epusdt/okpay/tokenpay）只支持 qr/redirect。
	// official provider 的 interaction_mode 验证由各 adapter 的 ValidateConfig 负责。
	if providerType != constants.PaymentProviderOfficial {
		mode := strings.ToLower(strings.TrimSpace(channel.InteractionMode))
		if mode != constants.PaymentInteractionQR && mode != constants.PaymentInteractionRedirect {
			return ErrPaymentChannelConfigInvalid
		}
	}

	if s.paymentProviderRegistry == nil {
		return ErrPaymentProviderNotSupported
	}
	p, ok := s.paymentProviderRegistry.Lookup(channel.ProviderType, channel.ChannelType)
	if !ok {
		return fmt.Errorf("%w: unsupported provider_type=%s channel_type=%s",
			ErrPaymentChannelConfigInvalid, channel.ProviderType, channel.ChannelType)
	}

	// official provider：第二参数传 interactionMode，供 wechatpay/alipay adapter 验证。
	// 非 official provider：第二参数传 channelType，供 epay/bepusdt/okpay adapter 验证 channel 类型。
	var validateParam string
	if providerType == constants.PaymentProviderOfficial {
		validateParam = strings.ToLower(strings.TrimSpace(channel.InteractionMode))
	} else {
		validateParam = channelType
	}
	if err := p.ValidateConfig(channel.ConfigJSON, validateParam); err != nil {
		return mapProviderErrorToService(err)
	}
	return nil
}

func resolveTokenPayOrderUserKey(order *models.Order) string {
	if order == nil {
		return ""
	}
	if order.UserID > 0 {
		return strconv.FormatUint(uint64(order.UserID), 10)
	}
	if guestEmail := strings.TrimSpace(order.GuestEmail); guestEmail != "" {
		return guestEmail
	}
	return strings.TrimSpace(order.OrderNo)
}
