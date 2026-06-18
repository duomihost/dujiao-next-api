package bepusdt

import (
	"testing"

	"github.com/dujiao-next/internal/constants"
)

func TestParseConfigAndNormalizeDefaults(t *testing.T) {
	cfg, err := ParseConfig(map[string]interface{}{
		"gateway_url": " https://pay.example.com/ ",
		"auth_token":  " token ",
		"notify_url":  " https://example.com/notify ",
		"return_url":  " https://example.com/return ",
	})
	if err != nil {
		t.Fatalf("parse config failed: %v", err)
	}
	if cfg.TradeType != bepusdtTradeTypeUSDTTRC20 {
		t.Fatalf("unexpected default trade type: %s", cfg.TradeType)
	}
	if cfg.Fiat != constants.SiteCurrencyDefault {
		t.Fatalf("unexpected default fiat: %s", cfg.Fiat)
	}
	if cfg.GatewayURL != "https://pay.example.com" {
		t.Fatalf("unexpected normalized gateway url: %s", cfg.GatewayURL)
	}
}

func TestResolveTradeType(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "USDT", input: bepusdtChannelTypeUSDT, expect: bepusdtTradeTypeUSDTTRC20},
		{name: "USDTTRC20", input: bepusdtChannelTypeUSDTTRC20, expect: bepusdtTradeTypeUSDTTRC20},
		{name: "USDCTRC20", input: bepusdtChannelTypeUSDCTRC20, expect: bepusdtTradeTypeUSDCTRC20},
		{name: "TRX", input: bepusdtChannelTypeTRX, expect: bepusdtTradeTypeTRX},
		{name: "Unknown", input: "unknown", expect: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveTradeType(tc.input); got != tc.expect {
				t.Fatalf("unexpected trade type: got %s, want %s", got, tc.expect)
			}
		})
	}
}

func TestToPaymentStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		expect string
	}{
		{name: "Success", status: StatusSuccess, expect: constants.PaymentStatusSuccess},
		{name: "Expired", status: StatusExpired, expect: constants.PaymentStatusExpired},
		{name: "Waiting", status: StatusWaiting, expect: constants.PaymentStatusPending},
		{name: "Unknown", status: 999, expect: constants.PaymentStatusPending},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToPaymentStatus(tc.status); got != tc.expect {
				t.Fatalf("unexpected payment status: got %s, want %s", got, tc.expect)
			}
		})
	}
}
