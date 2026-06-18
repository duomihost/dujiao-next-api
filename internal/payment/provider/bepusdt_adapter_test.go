package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/bepusdt"
)

func TestBepusdtAdapter_Type(t *testing.T) {
	a := NewBepusdtAdapter()
	want := constants.PaymentProviderBepusdt + ":"
	if got := a.Type(); got != want {
		t.Fatalf("Type() = %q, want %q", got, want)
	}
}

func TestBepusdtAdapter_ValidateConfig_UnsupportedChannel(t *testing.T) {
	a := NewBepusdtAdapter()
	err := a.ValidateConfig(models.JSON{}, "no-such-channel-type")
	if err == nil {
		t.Fatalf("expected error for unsupported channel")
	}
	if !errors.Is(err, ErrUnsupportedChannel) {
		t.Fatalf("expected wrapped ErrUnsupportedChannel, got %v", err)
	}
}

func TestBepusdtAdapter_CreatePayment_ConfigInvalidMapped(t *testing.T) {
	a := NewBepusdtAdapter()
	// 用 bepusdt 真实支持的 channelType（usdt-trc20 / usdc-trc20 / trx）
	_, err := a.CreatePayment(context.Background(), models.JSON{}, CreateInput{
		OrderNo:     "ORDER_1",
		Currency:    "USDT",
		ChannelType: "usdt-trc20",
	})
	if err == nil {
		t.Fatalf("expected error from empty config")
	}
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("expected wrapped ErrConfigInvalid, got %v", err)
	}
}

func TestBepusdtAdapter_MapBepusdtError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"config", bepusdt.ErrConfigInvalid, ErrConfigInvalid},
		{"trade_type→unsupported", bepusdt.ErrTradeTypeNotSupport, ErrUnsupportedChannel},
		{"request", bepusdt.ErrRequestFailed, ErrRequestFailed},
		{"response", bepusdt.ErrResponseInvalid, ErrResponseInvalid},
		{"signature", bepusdt.ErrSignatureInvalid, ErrSignatureInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapBepusdtError(tc.in)
			if !errors.Is(got, tc.want) {
				t.Fatalf("mapBepusdtError(%v) errors.Is %v = false, want true", tc.in, tc.want)
			}
		})
	}
}
