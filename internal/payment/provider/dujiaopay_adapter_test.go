package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
)

func TestDujiaoPayAdapter_Type(t *testing.T) {
	a := NewDujiaoPayAdapter()
	want := constants.PaymentProviderDujiaoPay + ":"
	if got := a.Type(); got != want {
		t.Fatalf("Type() = %q, want %q", got, want)
	}
}

func TestDujiaoPayAdapter_ValidateConfig_UnsupportedToken(t *testing.T) {
	a := NewDujiaoPayAdapter()
	err := a.ValidateConfig(models.JSON{
		"api_base_url":   "https://api.example.com",
		"api_key_id":     "key-1",
		"api_secret":     "secret-1",
		"webhook_secret": "whsec-1",
		"fiat_currency":  "USD",
	}, "doge-usdt")
	if err == nil {
		t.Fatalf("expected unsupported token error")
	}
	if !errors.Is(err, ErrUnsupportedChannel) {
		t.Fatalf("expected ErrUnsupportedChannel, got %v", err)
	}
}

func TestDujiaoPayAdapter_CreatePaymentQRCodeModeUsesWalletAddress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order_id":"do_1","chain":"tron","token_id":"tron-usdt","checkout_url":"https://pay.example.com/c/ct_1","pay_address":"TAddr","payable_amount":"10.0001","status":"pending"}`))
	}))
	defer server.Close()

	a := NewDujiaoPayAdapter()
	result, err := a.CreatePayment(context.Background(), models.JSON{
		"api_base_url":   server.URL,
		"api_key_id":     "key-1",
		"api_secret":     "secret-1",
		"webhook_secret": "whsec-1",
		"fiat_currency":  "USD",
	}, CreateInput{
		OrderNo:        "PAY-1",
		Amount:         models.NewMoneyFromDecimal(decimal.RequireFromString("10")),
		Currency:       "USD",
		ChannelType:    "tron-usdt",
		ReturnURLQuery: map[string]string{"biz_type": "order", "order_no": "ORDER-1"},
		Extra:          models.JSON{"interaction_mode": constants.PaymentInteractionQR},
	})
	if err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}
	if result.ProviderRef != "do_1" {
		t.Fatalf("ProviderRef = %q, want do_1", result.ProviderRef)
	}
	if result.RedirectURL != "https://pay.example.com/c/ct_1" {
		t.Fatalf("RedirectURL = %q", result.RedirectURL)
	}
	if result.QRCodeURL != "TAddr" {
		t.Fatalf("QRCodeURL = %q", result.QRCodeURL)
	}
	if result.Payload["pay_address"] != "TAddr" {
		t.Fatalf("payload pay_address = %v", result.Payload["pay_address"])
	}
	if result.Payload["chain"] != "tron" {
		t.Fatalf("payload chain = %v", result.Payload["chain"])
	}
	if result.Payload["token_id"] != "tron-usdt" {
		t.Fatalf("payload token_id = %v", result.Payload["token_id"])
	}
}
