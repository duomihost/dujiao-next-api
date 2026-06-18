package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/payment/tokenpay"
)

func TestTokenpayAdapter_Type(t *testing.T) {
	a := NewTokenpayAdapter()
	want := constants.PaymentProviderTokenpay + ":"
	if got := a.Type(); got != want {
		t.Fatalf("Type() = %q, want %q", got, want)
	}
}

func TestTokenpayAdapter_ValidateConfig_EmptyRejected(t *testing.T) {
	a := NewTokenpayAdapter()
	err := a.ValidateConfig(models.JSON{}, "")
	if err == nil {
		t.Fatalf("expected error from empty config")
	}
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("expected wrapped ErrConfigInvalid, got %v", err)
	}
}

func TestTokenpayAdapter_CreatePayment_ConfigInvalidMapped(t *testing.T) {
	a := NewTokenpayAdapter()
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

func TestTokenpayAdapter_MapTokenpayError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"config", tokenpay.ErrConfigInvalid, ErrConfigInvalid},
		{"request", tokenpay.ErrRequestFailed, ErrRequestFailed},
		{"response", tokenpay.ErrResponseInvalid, ErrResponseInvalid},
		{"signature", tokenpay.ErrSignatureInvalid, ErrSignatureInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapTokenpayError(tc.in)
			if !errors.Is(got, tc.want) {
				t.Fatalf("mapTokenpayError(%v) errors.Is %v = false, want true", tc.in, tc.want)
			}
		})
	}
}
