package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/epusdt"
)

func TestEpusdtAdapter_Type(t *testing.T) {
	a := NewEpusdtAdapter()
	want := constants.PaymentProviderEpusdt + ":"
	if got := a.Type(); got != want {
		t.Fatalf("Type() = %q, want %q", got, want)
	}
}

func TestEpusdtAdapter_ValidateConfig_EmptyRejected(t *testing.T) {
	a := NewEpusdtAdapter()
	err := a.ValidateConfig(models.JSON{}, "")
	if err == nil {
		t.Fatalf("expected error from empty config")
	}
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("expected wrapped ErrConfigInvalid, got %v", err)
	}
}

func TestEpusdtAdapter_CreatePayment_ConfigInvalidMapped(t *testing.T) {
	a := NewEpusdtAdapter()
	_, err := a.CreatePayment(context.Background(), models.JSON{}, CreateInput{
		OrderNo:  "ORDER_1",
		Currency: "USDT",
	})
	if err == nil {
		t.Fatalf("expected error from empty config")
	}
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("expected wrapped ErrConfigInvalid, got %v", err)
	}
}

func TestEpusdtAdapter_MapEpusdtError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"config", epusdt.ErrConfigInvalid, ErrConfigInvalid},
		{"request", epusdt.ErrRequestFailed, ErrRequestFailed},
		{"response", epusdt.ErrResponseInvalid, ErrResponseInvalid},
		{"signature", epusdt.ErrSignatureInvalid, ErrSignatureInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapEpusdtError(tc.in)
			if !errors.Is(got, tc.want) {
				t.Fatalf("mapEpusdtError(%v) errors.Is %v = false, want true", tc.in, tc.want)
			}
		})
	}
}
