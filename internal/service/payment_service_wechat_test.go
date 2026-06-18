package service

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/provider"

	"github.com/shopspring/decimal"
)

// buildMinimalPaymentServiceWithRegistry 构造一个只注入了 Registry 的 PaymentService，
// 供无需 DB 的 ValidateChannel 测试使用。
func buildMinimalPaymentServiceWithRegistry(t *testing.T) *PaymentService {
	t.Helper()
	reg := provider.NewRegistry()
	reg.Register(constants.PaymentProviderOfficial, constants.PaymentChannelTypeStripe, provider.NewStripeAdapter())
	reg.Register(constants.PaymentProviderOfficial, constants.PaymentChannelTypePaypal, provider.NewPaypalAdapter())
	reg.Register(constants.PaymentProviderOfficial, constants.PaymentChannelTypeWechat, provider.NewWechatpayAdapter())
	reg.Register(constants.PaymentProviderOfficial, constants.PaymentChannelTypeAlipay, provider.NewAlipayAdapter())
	reg.Register(constants.PaymentProviderEpay, "", provider.NewEpayAdapter())
	reg.Register(constants.PaymentProviderEpusdt, "", provider.NewEpusdtAdapter())
	reg.Register(constants.PaymentProviderBepusdt, "", provider.NewBepusdtAdapter())
	reg.Register(constants.PaymentProviderDujiaoPay, "", provider.NewDujiaoPayAdapter())
	reg.Register(constants.PaymentProviderTokenpay, "", provider.NewTokenpayAdapter())
	reg.Register(constants.PaymentProviderOkpay, "", provider.NewOkpayAdapter())
	return &PaymentService{paymentProviderRegistry: reg}
}

func TestValidateChannelWechatOfficial(t *testing.T) {
	svc := buildMinimalPaymentServiceWithRegistry(t)
	channel := &models.PaymentChannel{
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeWechat,
		InteractionMode: constants.PaymentInteractionRedirect,
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		ConfigJSON: models.JSON{
			"appid":                "wx1234567890",
			"mchid":                "1900000109",
			"merchant_serial_no":   "ABC123456789",
			"merchant_private_key": buildWechatTestPrivateKey(),
			"api_v3_key":           "12345678901234567890123456789012",
			"notify_url":           "https://example.com/api/v1/payments/callback",
			"h5_redirect_url":      "https://example.com/pay",
		},
	}
	if err := svc.ValidateChannel(channel); err != nil {
		t.Fatalf("validate wechat channel failed: %v", err)
	}
}

func TestValidateChannelWechatInvalidInteractionMode(t *testing.T) {
	svc := buildMinimalPaymentServiceWithRegistry(t)
	channel := &models.PaymentChannel{
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeWechat,
		InteractionMode: constants.PaymentInteractionWAP,
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		ConfigJSON: models.JSON{
			"appid":                "wx1234567890",
			"mchid":                "1900000109",
			"merchant_serial_no":   "ABC123456789",
			"merchant_private_key": buildWechatTestPrivateKey(),
			"api_v3_key":           "12345678901234567890123456789012",
			"notify_url":           "https://example.com/api/v1/payments/callback",
			"h5_redirect_url":      "https://example.com/pay",
		},
	}
	if err := svc.ValidateChannel(channel); err == nil {
		t.Fatalf("expected invalid interaction mode error")
	}
}

func buildWechatTestPrivateKey() string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}))
}

func TestShouldUseCNYPaymentCurrency(t *testing.T) {
	if shouldUseCNYPaymentCurrency(nil) {
		t.Fatalf("nil channel should not force CNY")
	}
	if !shouldUseCNYPaymentCurrency(&models.PaymentChannel{ProviderType: constants.PaymentProviderOfficial, ChannelType: constants.PaymentChannelTypeWechat}) {
		t.Fatalf("official wechat should force CNY")
	}
	if !shouldUseCNYPaymentCurrency(&models.PaymentChannel{ProviderType: constants.PaymentProviderOfficial, ChannelType: constants.PaymentChannelTypeAlipay}) {
		t.Fatalf("official alipay should force CNY")
	}
	if shouldUseCNYPaymentCurrency(&models.PaymentChannel{ProviderType: constants.PaymentProviderOfficial, ChannelType: constants.PaymentChannelTypePaypal}) {
		t.Fatalf("official paypal should not force CNY")
	}
}
