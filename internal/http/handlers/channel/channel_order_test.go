package channel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func TestBuildChannelOrderPreviewResponseIncludesTelegramFriendlyFields(t *testing.T) {
	resp := buildChannelOrderPreviewResponse(&service.OrderPreview{
		Currency:                "CNY",
		OriginalAmount:          models.NewMoneyFromDecimal(decimal.RequireFromString("108.00")),
		DiscountAmount:          models.NewMoneyFromDecimal(decimal.RequireFromString("8.00")),
		PromotionDiscountAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("10.00")),
		TotalAmount:             models.NewMoneyFromDecimal(decimal.RequireFromString("90.00")),
		Items: []service.OrderPreviewItem{{
			ProductID:          12,
			SKUID:              34,
			TitleJSON:          models.JSON{"zh-CN": "会员订阅"},
			SKUSnapshotJSON:    models.JSON{"spec_values": models.JSON{"zh-CN": "季度版"}},
			Quantity:           2,
			OriginalUnitPrice:  models.NewMoneyFromDecimal(decimal.RequireFromString("60.00")),
			UnitPrice:          models.NewMoneyFromDecimal(decimal.RequireFromString("54.00")),
			OriginalTotalPrice: models.NewMoneyFromDecimal(decimal.RequireFromString("120.00")),
			TotalPrice:         models.NewMoneyFromDecimal(decimal.RequireFromString("108.00")),
			CouponDiscount:     models.NewMoneyFromDecimal(decimal.RequireFromString("8.00")),
			PromotionDiscount:  models.NewMoneyFromDecimal(decimal.RequireFromString("10.00")),
			FulfillmentType:    "manual",
		}},
	}, "zh-CN")

	if got := resp["item_count"]; got != 1 {
		t.Fatalf("expected item_count=1, got=%v", got)
	}
	if got := resp["original_amount"]; got != "108.00" {
		t.Fatalf("expected original_amount=108.00, got=%v", got)
	}
	items, ok := resp["items"].([]gin.H)
	if !ok || len(items) != 1 {
		t.Fatalf("expected single preview item, got=%T len=%d", resp["items"], len(items))
	}
	if got := items[0]["coupon_discount"]; got != "8.00" {
		t.Fatalf("expected coupon_discount=8.00, got=%v", got)
	}
	if got := items[0]["original_unit_price"]; got != "60.00" {
		t.Fatalf("expected original_unit_price=60.00, got=%v", got)
	}
	if got := items[0]["original_total_price"]; got != "120.00" {
		t.Fatalf("expected original_total_price=120.00, got=%v", got)
	}
	if got := items[0]["promotion_discount"]; got != "10.00" {
		t.Fatalf("expected promotion_discount=10.00, got=%v", got)
	}
	if got := items[0]["fulfillment_type"]; got != "manual" {
		t.Fatalf("expected fulfillment_type=manual, got=%v", got)
	}
}

func TestBuildChannelOrderDetailResponseUsesTotalPaidAmount(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	resp := buildChannelOrderDetailResponse(&models.Order{
		ID:                      7,
		OrderNo:                 "DJ20260310001",
		Status:                  "paid",
		Currency:                "CNY",
		OriginalAmount:          models.NewMoneyFromDecimal(decimal.RequireFromString("100.00")),
		DiscountAmount:          models.NewMoneyFromDecimal(decimal.RequireFromString("5.00")),
		PromotionDiscountAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("15.00")),
		TotalAmount:             models.NewMoneyFromDecimal(decimal.RequireFromString("80.00")),
		WalletPaidAmount:        models.NewMoneyFromDecimal(decimal.RequireFromString("20.00")),
		OnlinePaidAmount:        models.NewMoneyFromDecimal(decimal.RequireFromString("60.00")),
		RefundedAmount:          models.NewMoneyFromDecimal(decimal.RequireFromString("0.00")),
		ExpiresAt:               &now,
		CreatedAt:               now,
		UpdatedAt:               now,
		PaidAt:                  &now,
		Items: []models.OrderItem{{
			ProductID:          1,
			SKUID:              2,
			TitleJSON:          models.JSON{"zh-CN": "测试商品"},
			SKUSnapshotJSON:    models.JSON{"spec_values": models.JSON{"zh-CN": "标准版"}},
			Quantity:           1,
			OriginalUnitPrice:  models.NewMoneyFromDecimal(decimal.RequireFromString("100.00")),
			UnitPrice:          models.NewMoneyFromDecimal(decimal.RequireFromString("80.00")),
			OriginalTotalPrice: models.NewMoneyFromDecimal(decimal.RequireFromString("100.00")),
			TotalPrice:         models.NewMoneyFromDecimal(decimal.RequireFromString("80.00")),
			CouponDiscount:     models.NewMoneyFromDecimal(decimal.RequireFromString("5.00")),
			PromotionDiscount:  models.NewMoneyFromDecimal(decimal.RequireFromString("15.00")),
			FulfillmentType:    "manual",
		}},
		Children: []models.Order{{
			ID:      8,
			OrderNo: "DJ20260310001-01",
			Status:  "completed",
			Fulfillment: &models.Fulfillment{
				Status:      "delivered",
				Type:        "auto",
				Payload:     "card-secret-demo",
				DeliveredAt: &now,
			},
		}},
	}, "zh-CN")

	if got := resp["paid_amount"]; got != "80.00" {
		t.Fatalf("expected paid_amount=80.00, got=%v", got)
	}
	if got := resp["wallet_paid_amount"]; got != "20.00" {
		t.Fatalf("expected wallet_paid_amount=20.00, got=%v", got)
	}
	if got := resp["online_paid_amount"]; got != "60.00" {
		t.Fatalf("expected online_paid_amount=60.00, got=%v", got)
	}
	if got := resp["item_count"]; got != 1 {
		t.Fatalf("expected item_count=1, got=%v", got)
	}
	if got := resp["fulfillment_type"]; got != "manual" {
		t.Fatalf("expected fulfillment_type=manual, got=%v", got)
	}
	items, ok := resp["items"].([]gin.H)
	if !ok || len(items) != 1 {
		t.Fatalf("expected single order item, got=%T len=%d", resp["items"], len(items))
	}
	if got := items[0]["coupon_discount"]; got != "5.00" {
		t.Fatalf("expected coupon_discount=5.00, got=%v", got)
	}
	if got := items[0]["original_unit_price"]; got != "100.00" {
		t.Fatalf("expected original_unit_price=100.00, got=%v", got)
	}
	if got := items[0]["original_total_price"]; got != "100.00" {
		t.Fatalf("expected original_total_price=100.00, got=%v", got)
	}
	if got := items[0]["promotion_discount"]; got != "15.00" {
		t.Fatalf("expected promotion_discount=15.00, got=%v", got)
	}
	if got := resp["fulfillment_delivered_at"]; got != nil {
		t.Fatalf("expected parent fulfillment_delivered_at=nil, got=%v", got)
	}
	children, ok := resp["children"].([]gin.H)
	if !ok || len(children) != 1 {
		t.Fatalf("expected single child order, got=%T len=%d", resp["children"], len(children))
	}
	childFulfillment, ok := children[0]["fulfillment"].(gin.H)
	if !ok {
		t.Fatalf("expected child fulfillment payload, got=%T", children[0]["fulfillment"])
	}
	if got := childFulfillment["payload"]; got != "card-secret-demo" {
		t.Fatalf("expected child fulfillment payload=card-secret-demo, got=%v", got)
	}
	if got := childFulfillment["status"]; got != "delivered" {
		t.Fatalf("expected child fulfillment status=delivered, got=%v", got)
	}
}

func TestBuildChannelPaymentResponseIncludesOrderSummary(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 30, 0, 0, time.UTC)
	order := &models.Order{
		ID:               9,
		OrderNo:          "DJ20260310002",
		TotalAmount:      models.NewMoneyFromDecimal(decimal.RequireFromString("99.00")),
		WalletPaidAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("19.00")),
		OnlinePaidAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("80.00")),
	}
	payment := &models.Payment{
		ID:              5,
		OrderID:         order.ID,
		ChannelID:       11,
		Status:          "pending",
		ProviderType:    "alipay",
		ChannelType:     "alipay",
		InteractionMode: "redirect",
		Amount:          models.NewMoneyFromDecimal(decimal.RequireFromString("80.80")),
		FeeRate:         models.NewMoneyFromDecimal(decimal.RequireFromString("1.00")),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.RequireFromString("0.80")),
		Currency:        "CNY",
		PayURL:          "https://pay.example.com",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	resp := buildChannelPaymentResponse(order, payment)
	if got := resp["channel_id"]; got != uint(11) {
		t.Fatalf("expected channel_id=11, got=%v", got)
	}
	if got := resp["fee_amount"]; got != "0.80" {
		t.Fatalf("expected fee_amount=0.80, got=%v", got)
	}
	if got := resp["order_no"]; got != "DJ20260310002" {
		t.Fatalf("expected order_no=DJ20260310002, got=%v", got)
	}
	if got := resp["paid_amount"]; got != "99.00" {
		t.Fatalf("expected paid_amount=99.00, got=%v", got)
	}
}

func TestPreviewOrderRequestBindsAffiliateFields(t *testing.T) {
	raw := []byte(`{
		"channel_user_id":"998877",
		"telegram_user_id":"998877",
		"items":[{"product_id":12,"sku_id":34,"quantity":1,"fulfillment_type":"manual"}],
		"affiliate_code":"AFFTG001",
		"affiliate_visitor_key":"visitor-998877"
	}`)

	var req previewOrderRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal preview order request failed: %v", err)
	}
	if req.AffiliateCode != "AFFTG001" {
		t.Fatalf("expected affiliate code to bind, got=%s", req.AffiliateCode)
	}
	if req.AffiliateKey != "visitor-998877" {
		t.Fatalf("expected affiliate visitor key to bind, got=%s", req.AffiliateKey)
	}
}

func TestCreateOrderRequestBindsAffiliateFields(t *testing.T) {
	raw := []byte(`{
		"channel_user_id":"556677",
		"telegram_user_id":"556677",
		"product_id":12,
		"sku_id":34,
		"quantity":2,
		"affiliate_code":"AFFTG002",
		"affiliate_visitor_key":"visitor-556677"
	}`)

	var req createOrderRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal create order request failed: %v", err)
	}
	if req.AffiliateCode != "AFFTG002" {
		t.Fatalf("expected affiliate code to bind, got=%s", req.AffiliateCode)
	}
	if req.AffiliateKey != "visitor-556677" {
		t.Fatalf("expected affiliate visitor key to bind, got=%s", req.AffiliateKey)
	}
}

func TestBuildChannelPaymentResponse_ProviderModeMatrix(t *testing.T) {
	type wantUSDT struct {
		address string
		amount  string
		chain   string
		tokenID string
	}
	tests := []struct {
		name            string
		providerType    string
		channelType     string
		interactionMode string
		payload         models.JSON
		wantUSDT        *wantUSDT // nil => no wallet_address/chain_amount keys expected
	}{
		{name: "alipay qr", providerType: constants.PaymentProviderOfficial, channelType: constants.PaymentChannelTypeAlipay, interactionMode: constants.PaymentInteractionQR},
		{name: "alipay wap", providerType: constants.PaymentProviderOfficial, channelType: constants.PaymentChannelTypeAlipay, interactionMode: constants.PaymentInteractionWAP},
		{name: "alipay page", providerType: constants.PaymentProviderOfficial, channelType: constants.PaymentChannelTypeAlipay, interactionMode: constants.PaymentInteractionPage},
		{name: "wechat qr", providerType: constants.PaymentProviderOfficial, channelType: constants.PaymentChannelTypeWechat, interactionMode: constants.PaymentInteractionQR},
		{name: "wechat redirect", providerType: constants.PaymentProviderOfficial, channelType: constants.PaymentChannelTypeWechat, interactionMode: constants.PaymentInteractionRedirect},
		{name: "epay qr", providerType: constants.PaymentProviderEpay, channelType: "", interactionMode: constants.PaymentInteractionQR},
		{name: "epay redirect", providerType: constants.PaymentProviderEpay, channelType: "", interactionMode: constants.PaymentInteractionRedirect},
		{
			name:            "bepusdt qr with usdt payload",
			providerType:    constants.PaymentProviderBepusdt,
			channelType:     "",
			interactionMode: constants.PaymentInteractionQR,
			payload:         models.JSON{"data": map[string]any{"token": "TBepAddr", "actual_amount": "13.45"}},
			wantUSDT:        &wantUSDT{address: "TBepAddr", amount: "13.45"},
		},
		{name: "bepusdt redirect", providerType: constants.PaymentProviderBepusdt, channelType: "", interactionMode: constants.PaymentInteractionRedirect, payload: models.JSON{"data": map[string]any{"token": "TBepAddr", "actual_amount": "13.45"}}},
		{
			name:            "epusdt qr with receive_address payload",
			providerType:    constants.PaymentProviderEpusdt,
			channelType:     "",
			interactionMode: constants.PaymentInteractionQR,
			payload:         models.JSON{"data": map[string]any{"receive_address": "TEpusdtAddr", "actual_amount": "9.99"}},
			wantUSDT:        &wantUSDT{address: "TEpusdtAddr", amount: "9.99"},
		},
		{
			name:            "epusdt qr without receive_address still surfaces chain amount",
			providerType:    constants.PaymentProviderEpusdt,
			channelType:     "",
			interactionMode: constants.PaymentInteractionQR,
			payload:         models.JSON{"data": map[string]any{"actual_amount": "5.00"}},
			wantUSDT:        &wantUSDT{address: "", amount: "5.00"},
		},
		{name: "epusdt redirect", providerType: constants.PaymentProviderEpusdt, channelType: "", interactionMode: constants.PaymentInteractionRedirect},
		{
			name:            "dujiaopay qr with wallet payload",
			providerType:    constants.PaymentProviderDujiaoPay,
			channelType:     "tron-usdt",
			interactionMode: constants.PaymentInteractionQR,
			payload: models.JSON{
				"chain":          "tron",
				"token_id":       "tron-usdt",
				"pay_address":    "TDujiaoAddr",
				"payable_amount": "10.0001",
			},
			wantUSDT: &wantUSDT{address: "TDujiaoAddr", amount: "10.0001", chain: "tron", tokenID: "tron-usdt"},
		},
		{name: "okpay qr", providerType: constants.PaymentProviderOkpay, channelType: "", interactionMode: constants.PaymentInteractionQR},
		{name: "okpay redirect", providerType: constants.PaymentProviderOkpay, channelType: "", interactionMode: constants.PaymentInteractionRedirect},
		{name: "stripe redirect", providerType: constants.PaymentProviderOfficial, channelType: constants.PaymentChannelTypeStripe, interactionMode: constants.PaymentInteractionRedirect},
		{name: "paypal redirect", providerType: constants.PaymentProviderOfficial, channelType: constants.PaymentChannelTypePaypal, interactionMode: constants.PaymentInteractionRedirect},
		{name: "tokenpay qr", providerType: constants.PaymentProviderTokenpay, channelType: "", interactionMode: constants.PaymentInteractionQR},
		{name: "tokenpay redirect", providerType: constants.PaymentProviderTokenpay, channelType: "", interactionMode: constants.PaymentInteractionRedirect},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			channelType := tc.channelType
			if channelType == "" {
				channelType = "test-channel-type"
			}
			payment := &models.Payment{
				ID:              1,
				OrderID:         2,
				ChannelID:       3,
				ProviderType:    tc.providerType,
				ChannelType:     channelType,
				InteractionMode: tc.interactionMode,
				Status:          constants.PaymentStatusPending,
				Amount:          models.NewMoneyFromDecimal(decimal.RequireFromString("100")),
				FeeRate:         models.Money{},
				FeeAmount:       models.Money{},
				Currency:        "CNY",
				PayURL:          "https://pay.example.com/c/abc",
				QRCode:          "https://pay.example.com/c/abc-qr",
				ProviderPayload: tc.payload,
			}
			resp := buildChannelPaymentResponse(nil, payment)
			if got := resp["interaction_mode"]; got != tc.interactionMode {
				t.Errorf("interaction_mode: got %v want %v", got, tc.interactionMode)
			}
			if got := resp["pay_url"]; got != "https://pay.example.com/c/abc" {
				t.Errorf("pay_url: got %v", got)
			}
			if got := resp["qr_code"]; got != "https://pay.example.com/c/abc-qr" {
				t.Errorf("qr_code: got %v", got)
			}
			if tc.wantUSDT == nil {
				if _, ok := resp["wallet_address"]; ok {
					t.Errorf("wallet_address should be absent, got %v", resp["wallet_address"])
				}
				if _, ok := resp["chain_amount"]; ok {
					t.Errorf("chain_amount should be absent, got %v", resp["chain_amount"])
				}
				if _, ok := resp["chain"]; ok {
					t.Errorf("chain should be absent, got %v", resp["chain"])
				}
				if _, ok := resp["token_id"]; ok {
					t.Errorf("token_id should be absent, got %v", resp["token_id"])
				}
				return
			}
			if tc.wantUSDT.address == "" {
				if _, ok := resp["wallet_address"]; ok {
					t.Errorf("wallet_address should be absent, got %v", resp["wallet_address"])
				}
			} else if got := resp["wallet_address"]; got != tc.wantUSDT.address {
				t.Errorf("wallet_address: got %v want %v", got, tc.wantUSDT.address)
			}
			if tc.wantUSDT.amount == "" {
				if _, ok := resp["chain_amount"]; ok {
					t.Errorf("chain_amount should be absent, got %v", resp["chain_amount"])
				}
			} else if got := resp["chain_amount"]; got != tc.wantUSDT.amount {
				t.Errorf("chain_amount: got %v want %v", got, tc.wantUSDT.amount)
			}
			if tc.wantUSDT.chain == "" {
				if _, ok := resp["chain"]; ok {
					t.Errorf("chain should be absent, got %v", resp["chain"])
				}
			} else if got := resp["chain"]; got != tc.wantUSDT.chain {
				t.Errorf("chain: got %v want %v", got, tc.wantUSDT.chain)
			}
			if tc.wantUSDT.tokenID == "" {
				if _, ok := resp["token_id"]; ok {
					t.Errorf("token_id should be absent, got %v", resp["token_id"])
				}
			} else if got := resp["token_id"]; got != tc.wantUSDT.tokenID {
				t.Errorf("token_id: got %v want %v", got, tc.wantUSDT.tokenID)
			}
		})
	}
}

func TestBuildChannelPaymentResponse_USDTQRExposesWalletFields(t *testing.T) {
	payment := &models.Payment{
		ID:              42,
		OrderID:         1,
		ChannelID:       2,
		ProviderType:    constants.PaymentProviderBepusdt,
		ChannelType:     "usdt-trc20",
		InteractionMode: constants.PaymentInteractionQR,
		Status:          constants.PaymentStatusPending,
		Amount:          models.NewMoneyFromDecimal(decimal.RequireFromString("100.00")),
		FeeRate:         models.Money{},
		FeeAmount:       models.Money{},
		Currency:        "CNY",
		PayURL:          "https://pay.example.com/c/abc",
		QRCode:          "https://pay.example.com/c/abc",
		ProviderPayload: models.JSON{
			"data": map[string]any{
				"token":         "TXxxxxx",
				"actual_amount": "13.45",
			},
		},
	}
	resp := buildChannelPaymentResponse(nil, payment)
	if got := resp["wallet_address"]; got != "TXxxxxx" {
		t.Fatalf("wallet_address: got %v want TXxxxxx", got)
	}
	if got := resp["chain_amount"]; got != "13.45" {
		t.Fatalf("chain_amount: got %v want 13.45", got)
	}
	if got := resp["interaction_mode"]; got != constants.PaymentInteractionQR {
		t.Fatalf("interaction_mode: got %v want qr", got)
	}
}
