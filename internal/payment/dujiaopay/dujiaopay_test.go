package dujiaopay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignHeadersBuildsDujiaoPayCanonicalSignature(t *testing.T) {
	body := []byte(`{"chain":"tron","token_id":"tron-usdt","fiat_currency":"USD","fiat_amount":"20"}`)

	headers := SignHeaders("secret-1", "key-1", "POST", "/v1/orders", "b=2&a=1", body, 1750000000, "nonce-1")

	sum := sha256.Sum256(body)
	canonical := strings.Join([]string{
		"POST",
		"/v1/orders",
		"a=1&b=2",
		hex.EncodeToString(sum[:]),
		"1750000000",
		"nonce-1",
	}, "\n")
	mac := hmac.New(sha256.New, []byte("secret-1"))
	mac.Write([]byte(canonical))
	wantSig := hex.EncodeToString(mac.Sum(nil))

	if headers["DJP-Key-ID"] != "key-1" {
		t.Fatalf("DJP-Key-ID = %q, want key-1", headers["DJP-Key-ID"])
	}
	if headers["DJP-Timestamp"] != "1750000000" {
		t.Fatalf("DJP-Timestamp = %q, want 1750000000", headers["DJP-Timestamp"])
	}
	if headers["DJP-Nonce"] != "nonce-1" {
		t.Fatalf("DJP-Nonce = %q, want nonce-1", headers["DJP-Nonce"])
	}
	if headers["DJP-Signature"] != wantSig {
		t.Fatalf("DJP-Signature = %q, want %q", headers["DJP-Signature"], wantSig)
	}
}

func TestCreatePaymentPostsSignedDujiaoPayOrder(t *testing.T) {
	now := time.Unix(1750000000, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/orders" {
			t.Fatalf("path = %s, want /v1/orders", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["merchant_order_id"] != "PAY-1001" {
			t.Fatalf("merchant_order_id = %v, want PAY-1001", payload["merchant_order_id"])
		}
		if payload["chain"] != "tron" || payload["token_id"] != "tron-usdt" {
			t.Fatalf("unexpected chain/token payload: %+v", payload)
		}
		if payload["fiat_currency"] != "USD" || payload["fiat_amount"] != "20.00" {
			t.Fatalf("unexpected fiat payload: %+v", payload)
		}
		if r.Header.Get("DJP-Key-ID") != "key-1" {
			t.Fatalf("DJP-Key-ID = %q", r.Header.Get("DJP-Key-ID"))
		}
		if r.Header.Get("DJP-Timestamp") != "1750000000" {
			t.Fatalf("DJP-Timestamp = %q", r.Header.Get("DJP-Timestamp"))
		}
		if r.Header.Get("DJP-Nonce") != "nonce-1" {
			t.Fatalf("DJP-Nonce = %q", r.Header.Get("DJP-Nonce"))
		}
		if r.Header.Get("Idempotency-Key") != "PAY-1001" {
			t.Fatalf("Idempotency-Key = %q", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"order_id":"do_1001",
			"chain":"tron",
			"token_id":"tron-usdt",
			"pay_address":"TAddress",
			"payable_amount":"20.0001",
			"status":"pending",
			"expires_at":"2026-06-11T00:15:00Z",
			"checkout_token":"ct_once",
			"checkout_url":"https://pay.example.com/c/ct_once"
		}`))
	}))
	defer server.Close()

	result, err := CreatePayment(context.Background(), &Config{
		APIBaseURL:    server.URL,
		APIKeyID:      "key-1",
		APISecret:     "secret-1",
		WebhookSecret: "whsec-1",
		Chain:         "tron",
		TokenID:       "tron-usdt",
		FiatCurrency:  "USD",
	}, CreateInput{
		MerchantOrderID: "PAY-1001",
		FiatAmount:      "20.00",
		SuccessURL:      "https://shop.example.com/pay?status=success",
		CancelURL:       "https://shop.example.com/pay?status=cancel",
	}, WithNowFunc(func() time.Time { return now }), WithNonceFunc(func() string { return "nonce-1" }))
	if err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}
	if result.OrderID != "do_1001" {
		t.Fatalf("OrderID = %q, want do_1001", result.OrderID)
	}
	if result.CheckoutURL != "https://pay.example.com/c/ct_once" {
		t.Fatalf("CheckoutURL = %q", result.CheckoutURL)
	}
	if result.PayableAmount != "20.0001" || result.PayAddress != "TAddress" {
		t.Fatalf("unexpected payment details: %+v", result)
	}
}

func TestParseWebhookVerifiesSignatureAndMapsPaidEvent(t *testing.T) {
	body := []byte(`{"event_id":"evt_1","event_type":"order.paid","event_version":"v1","created_at":"2026-06-06T12:00:00Z","data":{"order_id":"do_1001","merchant_order_id":"PAY-1001","tx_hash":"0xabc"}}`)
	mac := hmac.New(sha256.New, []byte("whsec-1"))
	mac.Write([]byte("1750000000."))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	event, err := ParseWebhook(&Config{WebhookSecret: "whsec-1"}, map[string]string{
		"DJP-Webhook-ID":        "evt_1",
		"DJP-Webhook-Timestamp": "1750000000",
		"DJP-Webhook-Signature": signature,
	}, body, time.Unix(1750000010, 0))
	if err != nil {
		t.Fatalf("ParseWebhook failed: %v", err)
	}
	if event.EventType != "order.paid" {
		t.Fatalf("EventType = %q", event.EventType)
	}
	if event.Status != "success" {
		t.Fatalf("Status = %q, want success", event.Status)
	}
	if event.OrderID != "do_1001" || event.MerchantOrderID != "PAY-1001" {
		t.Fatalf("unexpected ids: %+v", event)
	}
	if event.TxHash != "0xabc" {
		t.Fatalf("TxHash = %q, want 0xabc", event.TxHash)
	}
}
