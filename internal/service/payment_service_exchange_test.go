package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// --- Integration tests: exchange rate with callback verification ---

func setupExchangeTest(t *testing.T) (*PaymentService, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:payment_exchange_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Order{},
		&models.OrderItem{},
		&models.Fulfillment{},
		&models.Product{},
		&models.ProductSKU{},
		&models.WalletAccount{},
		&models.WalletTransaction{},
		&models.WalletRechargeOrder{},
		&models.PaymentChannel{},
		&models.Payment{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	models.DB = db

	orderRepo := repository.NewOrderRepository(db)
	productRepo := repository.NewProductRepository(db)
	productSKURepo := repository.NewProductSKURepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	channelRepo := repository.NewPaymentChannelRepository(db)
	walletRepo := repository.NewWalletRepository(db)
	userRepo := repository.NewUserRepository(db)
	refundRecordRepo := repository.NewOrderRefundRecordRepository(db)
	walletSvc := NewWalletService(walletRepo, orderRepo, refundRecordRepo, userRepo, nil, nil)
	paymentSvc := NewPaymentService(PaymentServiceOptions{
		OrderRepo:      orderRepo,
		ProductRepo:    productRepo,
		ProductSKURepo: productSKURepo,
		PaymentRepo:    paymentRepo,
		ChannelRepo:    channelRepo,
		WalletRepo:     walletRepo,
		WalletService:  walletSvc,
		ExpireMinutes:  15,
	})
	return paymentSvc, db
}

func createExchangePaymentFixture(t *testing.T, db *gorm.DB, originalAmount decimal.Decimal, originalCurrency string, convertedAmount decimal.Decimal, convertedCurrency string, exchangeRate string) (*models.Payment, *models.Order) {
	t.Helper()
	now := time.Now()

	user := &models.User{
		Email:        fmt.Sprintf("exchange_test_%d@example.com", now.UnixNano()),
		PasswordHash: "hash",
		Status:       constants.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	order := &models.Order{
		OrderNo:          fmt.Sprintf("DJEXTEST%d", now.UnixNano()),
		UserID:           user.ID,
		Status:           constants.OrderStatusPendingPayment,
		Currency:         originalCurrency,
		OriginalAmount:   models.NewMoneyFromDecimal(originalAmount),
		TotalAmount:      models.NewMoneyFromDecimal(originalAmount),
		OnlinePaidAmount: models.NewMoneyFromDecimal(decimal.Zero),
		WalletPaidAmount: models.NewMoneyFromDecimal(decimal.Zero),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	payment := &models.Payment{
		OrderID:         order.ID,
		ChannelID:       1,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeAlipay,
		InteractionMode: constants.PaymentInteractionQR,
		Amount:          models.NewMoneyFromDecimal(convertedAmount),
		Currency:        convertedCurrency,
		FeeRate:         models.NewMoneyFromDecimal(decimal.Zero),
		FixedFee:        models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount:       models.NewMoneyFromDecimal(decimal.Zero),
		Status:          constants.PaymentStatusPending,
		ProviderRef:     fmt.Sprintf("PAY-%d", now.UnixNano()),
		GatewayOrderNo:  order.OrderNo,
		ProviderPayload: models.JSON{
			"exchange_rate":     exchangeRate,
			"original_amount":   originalAmount.StringFixed(2),
			"original_currency": originalCurrency,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(payment).Error; err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	return payment, order
}

func TestCallbackMatchesConvertedAmount(t *testing.T) {
	svc, db := setupExchangeTest(t)
	// Order: $10 USD, converted to ¥72 CNY (rate 7.2)
	payment, order := createExchangePaymentFixture(t, db,
		decimal.NewFromInt(10), "USD",
		decimal.RequireFromString("72"), "CNY",
		"7.2",
	)

	now := time.Now()
	updated, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     order.OrderNo,
		ChannelID:   payment.ChannelID,
		Status:      constants.PaymentStatusSuccess,
		ProviderRef: "ALIPAY-SUCCESS-001",
		Amount:      models.NewMoneyFromDecimal(decimal.RequireFromString("72")),
		Currency:    "CNY",
		PaidAt:      &now,
	})
	if err != nil {
		t.Fatalf("callback should succeed with converted amount: %v", err)
	}
	if updated.Status != constants.PaymentStatusSuccess {
		t.Errorf("status = %s, want %s", updated.Status, constants.PaymentStatusSuccess)
	}
}

func TestCallbackRejectsOriginalAmountWhenConverted(t *testing.T) {
	svc, db := setupExchangeTest(t)
	// Payment stored as ¥72 CNY (converted from $10 USD)
	payment, order := createExchangePaymentFixture(t, db,
		decimal.NewFromInt(10), "USD",
		decimal.RequireFromString("72"), "CNY",
		"7.2",
	)

	now := time.Now()
	_, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     order.OrderNo,
		ChannelID:   payment.ChannelID,
		Status:      constants.PaymentStatusSuccess,
		ProviderRef: "ALIPAY-FAIL-001",
		Amount:      models.NewMoneyFromDecimal(decimal.NewFromInt(10)), // original USD amount
		Currency:    "CNY",
		PaidAt:      &now,
	})
	if err != ErrPaymentAmountMismatch {
		t.Fatalf("callback with original amount should fail with ErrPaymentAmountMismatch, got: %v", err)
	}
}

func TestCallbackRejectsCurrencyMismatchAfterConversion(t *testing.T) {
	svc, db := setupExchangeTest(t)
	payment, order := createExchangePaymentFixture(t, db,
		decimal.NewFromInt(10), "USD",
		decimal.RequireFromString("72"), "CNY",
		"7.2",
	)

	now := time.Now()
	_, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     order.OrderNo,
		ChannelID:   payment.ChannelID,
		Status:      constants.PaymentStatusSuccess,
		ProviderRef: "REF-001",
		Amount:      models.NewMoneyFromDecimal(decimal.RequireFromString("72")),
		Currency:    "USD", // wrong currency
		PaidAt:      &now,
	})
	if err != ErrPaymentCurrencyMismatch {
		t.Fatalf("callback with wrong currency should fail with ErrPaymentCurrencyMismatch, got: %v", err)
	}
}

func TestCallbackSkipsAmountCheckWhenZero(t *testing.T) {
	svc, db := setupExchangeTest(t)
	payment, order := createExchangePaymentFixture(t, db,
		decimal.NewFromInt(10), "USD",
		decimal.RequireFromString("72"), "CNY",
		"7.2",
	)

	now := time.Now()
	updated, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     order.OrderNo,
		ChannelID:   payment.ChannelID,
		Status:      constants.PaymentStatusSuccess,
		ProviderRef: "REF-002",
		Amount:      models.NewMoneyFromDecimal(decimal.Zero), // zero = skip check
		Currency:    "CNY",
		PaidAt:      &now,
	})
	if err != nil {
		t.Fatalf("callback with zero amount should skip amount check: %v", err)
	}
	if updated.Status != constants.PaymentStatusSuccess {
		t.Errorf("status = %s, want %s", updated.Status, constants.PaymentStatusSuccess)
	}
}

func TestCallbackSkipsCurrencyCheckWhenEmpty(t *testing.T) {
	svc, db := setupExchangeTest(t)
	payment, order := createExchangePaymentFixture(t, db,
		decimal.NewFromInt(10), "USD",
		decimal.RequireFromString("72"), "CNY",
		"7.2",
	)

	now := time.Now()
	updated, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     order.OrderNo,
		ChannelID:   payment.ChannelID,
		Status:      constants.PaymentStatusSuccess,
		ProviderRef: "REF-003",
		Amount:      models.NewMoneyFromDecimal(decimal.RequireFromString("72")),
		Currency:    "", // empty = skip check
		PaidAt:      &now,
	})
	if err != nil {
		t.Fatalf("callback with empty currency should skip currency check: %v", err)
	}
	if updated.Status != constants.PaymentStatusSuccess {
		t.Errorf("status = %s, want %s", updated.Status, constants.PaymentStatusSuccess)
	}
}

func TestCallbackNoConversionPaymentStillWorks(t *testing.T) {
	svc, db := setupExchangeTest(t)
	now := time.Now()

	user := &models.User{
		Email: fmt.Sprintf("noconv_%d@example.com", now.UnixNano()), PasswordHash: "h", Status: constants.UserStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	order := &models.Order{
		OrderNo: fmt.Sprintf("DJNOCONV%d", now.UnixNano()), UserID: user.ID,
		Status: constants.OrderStatusPendingPayment, Currency: "USD",
		OriginalAmount:   models.NewMoneyFromDecimal(decimal.NewFromInt(25)),
		TotalAmount:      models.NewMoneyFromDecimal(decimal.NewFromInt(25)),
		OnlinePaidAmount: models.NewMoneyFromDecimal(decimal.Zero),
		WalletPaidAmount: models.NewMoneyFromDecimal(decimal.Zero),
		CreatedAt:        now, UpdatedAt: now,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}
	payment := &models.Payment{
		OrderID: order.ID, ChannelID: 1,
		ProviderType: constants.PaymentProviderOfficial, ChannelType: constants.PaymentChannelTypeStripe,
		InteractionMode: constants.PaymentInteractionRedirect,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(25)), Currency: "USD",
		FeeRate: models.NewMoneyFromDecimal(decimal.Zero), FixedFee: models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount: models.NewMoneyFromDecimal(decimal.Zero),
		Status:    constants.PaymentStatusPending, ProviderRef: "pi_test",
		GatewayOrderNo: order.OrderNo, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(payment).Error; err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	updated, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID: payment.ID, OrderNo: order.OrderNo, ChannelID: payment.ChannelID,
		Status: constants.PaymentStatusSuccess, ProviderRef: "pi_test",
		Amount: models.NewMoneyFromDecimal(decimal.NewFromInt(25)), Currency: "USD", PaidAt: &now,
	})
	if err != nil {
		t.Fatalf("callback without conversion should succeed: %v", err)
	}
	if updated.Status != constants.PaymentStatusSuccess {
		t.Errorf("status = %s, want %s", updated.Status, constants.PaymentStatusSuccess)
	}
}

func TestCallbackRejectsAmountMismatchWithoutConversion(t *testing.T) {
	svc, db := setupExchangeTest(t)
	now := time.Now()

	user := &models.User{
		Email: fmt.Sprintf("mismatch_%d@example.com", now.UnixNano()), PasswordHash: "h", Status: constants.UserStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	order := &models.Order{
		OrderNo: fmt.Sprintf("DJMISMATCH%d", now.UnixNano()), UserID: user.ID,
		Status: constants.OrderStatusPendingPayment, Currency: "USD",
		OriginalAmount:   models.NewMoneyFromDecimal(decimal.NewFromInt(25)),
		TotalAmount:      models.NewMoneyFromDecimal(decimal.NewFromInt(25)),
		OnlinePaidAmount: models.NewMoneyFromDecimal(decimal.Zero),
		WalletPaidAmount: models.NewMoneyFromDecimal(decimal.Zero),
		CreatedAt:        now, UpdatedAt: now,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}
	payment := &models.Payment{
		OrderID: order.ID, ChannelID: 1,
		ProviderType: constants.PaymentProviderOfficial, ChannelType: constants.PaymentChannelTypeStripe,
		InteractionMode: constants.PaymentInteractionRedirect,
		Amount:          models.NewMoneyFromDecimal(decimal.NewFromInt(25)), Currency: "USD",
		FeeRate: models.NewMoneyFromDecimal(decimal.Zero), FixedFee: models.NewMoneyFromDecimal(decimal.Zero),
		FeeAmount: models.NewMoneyFromDecimal(decimal.Zero),
		Status:    constants.PaymentStatusPending, ProviderRef: "pi_test2",
		GatewayOrderNo: order.OrderNo, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(payment).Error; err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	_, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID: payment.ID, OrderNo: order.OrderNo, ChannelID: payment.ChannelID,
		Status: constants.PaymentStatusSuccess, ProviderRef: "pi_test2",
		Amount: models.NewMoneyFromDecimal(decimal.NewFromInt(10)), Currency: "USD", PaidAt: &now,
	})
	if err != ErrPaymentAmountMismatch {
		t.Fatalf("callback with mismatched amount should fail, got: %v", err)
	}
}

func TestCallbackIdempotentSuccessWithConvertedAmount(t *testing.T) {
	svc, db := setupExchangeTest(t)
	payment, order := createExchangePaymentFixture(t, db,
		decimal.NewFromInt(10), "USD",
		decimal.RequireFromString("72"), "CNY",
		"7.2",
	)
	// Mark payment as already successful
	paidAt := time.Now()
	db.Model(payment).Updates(map[string]interface{}{
		"status":  constants.PaymentStatusSuccess,
		"paid_at": paidAt,
	})

	updated, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     order.OrderNo,
		ChannelID:   payment.ChannelID,
		Status:      constants.PaymentStatusSuccess,
		ProviderRef: "ALIPAY-IDEM-001",
		Amount:      models.NewMoneyFromDecimal(decimal.RequireFromString("72")),
		Currency:    "CNY",
		PaidAt:      &paidAt,
	})
	if err != nil {
		t.Fatalf("idempotent callback should succeed: %v", err)
	}
	if updated.Status != constants.PaymentStatusSuccess {
		t.Errorf("status = %s, want %s", updated.Status, constants.PaymentStatusSuccess)
	}
}

// --- Exchange rate conversion scenarios table test ---

func TestExchangeConversionScenarios(t *testing.T) {
	tests := []struct {
		name              string
		originalAmount    string
		originalCurrency  string
		convertedAmount   string
		convertedCurrency string
		exchangeRate      string
		callbackAmount    string
		callbackCurrency  string
		wantErr           error
	}{
		{
			name:           "alipay USD to CNY success",
			originalAmount: "10.00", originalCurrency: "USD",
			convertedAmount: "72.00", convertedCurrency: "CNY",
			exchangeRate:   "7.2",
			callbackAmount: "72.00", callbackCurrency: "CNY",
			wantErr: nil,
		},
		{
			name:           "wechat USD to CNY success",
			originalAmount: "5.50", originalCurrency: "USD",
			convertedAmount: "39.60", convertedCurrency: "CNY",
			exchangeRate:   "7.2",
			callbackAmount: "39.60", callbackCurrency: "CNY",
			wantErr: nil,
		},
		{
			name:           "stripe USD to EUR success",
			originalAmount: "100.00", originalCurrency: "USD",
			convertedAmount: "92.00", convertedCurrency: "EUR",
			exchangeRate:   "0.92",
			callbackAmount: "92.00", callbackCurrency: "EUR",
			wantErr: nil,
		},
		{
			name:           "epay USD to CNY success",
			originalAmount: "15.00", originalCurrency: "USD",
			convertedAmount: "108.00", convertedCurrency: "CNY",
			exchangeRate:   "7.2",
			callbackAmount: "108.00", callbackCurrency: "CNY",
			wantErr: nil,
		},
		{
			name:           "amount mismatch rejected",
			originalAmount: "10.00", originalCurrency: "USD",
			convertedAmount: "72.00", convertedCurrency: "CNY",
			exchangeRate:   "7.2",
			callbackAmount: "71.00", callbackCurrency: "CNY",
			wantErr: ErrPaymentAmountMismatch,
		},
		{
			name:           "currency mismatch rejected",
			originalAmount: "10.00", originalCurrency: "USD",
			convertedAmount: "72.00", convertedCurrency: "CNY",
			exchangeRate:   "7.2",
			callbackAmount: "72.00", callbackCurrency: "USD",
			wantErr: ErrPaymentCurrencyMismatch,
		},
		{
			name:           "original amount as callback rejected",
			originalAmount: "20.00", originalCurrency: "USD",
			convertedAmount: "144.00", convertedCurrency: "CNY",
			exchangeRate:   "7.2",
			callbackAmount: "20.00", callbackCurrency: "CNY",
			wantErr: ErrPaymentAmountMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, db := setupExchangeTest(t)
			payment, order := createExchangePaymentFixture(t, db,
				decimal.RequireFromString(tt.originalAmount), tt.originalCurrency,
				decimal.RequireFromString(tt.convertedAmount), tt.convertedCurrency,
				tt.exchangeRate,
			)

			now := time.Now()
			_, err := svc.HandleCallback(PaymentCallbackInput{
				PaymentID:   payment.ID,
				OrderNo:     order.OrderNo,
				ChannelID:   payment.ChannelID,
				Status:      constants.PaymentStatusSuccess,
				ProviderRef: fmt.Sprintf("REF-%d", now.UnixNano()),
				Amount:      models.NewMoneyFromDecimal(decimal.RequireFromString(tt.callbackAmount)),
				Currency:    tt.callbackCurrency,
				PaidAt:      &now,
			})

			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("want err %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// --- ProviderPayload audit data verification ---

func TestProviderPayloadRetainsOriginalAfterConversion(t *testing.T) {
	svc, db := setupExchangeTest(t)
	payment, order := createExchangePaymentFixture(t, db,
		decimal.NewFromInt(10), "USD",
		decimal.RequireFromString("72"), "CNY",
		"7.2",
	)

	now := time.Now()
	updated, err := svc.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     order.OrderNo,
		ChannelID:   payment.ChannelID,
		Status:      constants.PaymentStatusSuccess,
		ProviderRef: "AUDIT-001",
		Amount:      models.NewMoneyFromDecimal(decimal.RequireFromString("72")),
		Currency:    "CNY",
		PaidAt:      &now,
	})
	if err != nil {
		t.Fatalf("callback failed: %v", err)
	}

	// Reload from database to verify persisted data
	var dbPayment models.Payment
	if err := db.First(&dbPayment, updated.ID).Error; err != nil {
		t.Fatalf("reload payment failed: %v", err)
	}

	if dbPayment.Amount.String() != "72.00" {
		t.Errorf("persisted Amount = %s, want 72.00", dbPayment.Amount.String())
	}
	if dbPayment.Currency != "CNY" {
		t.Errorf("persisted Currency = %s, want CNY", dbPayment.Currency)
	}

	// Verify audit trail in ProviderPayload
	if dbPayment.ProviderPayload == nil {
		t.Fatal("ProviderPayload should not be nil")
	}
	if dbPayment.ProviderPayload["original_amount"] != "10.00" {
		t.Errorf("original_amount = %v, want 10.00", dbPayment.ProviderPayload["original_amount"])
	}
	if dbPayment.ProviderPayload["original_currency"] != "USD" {
		t.Errorf("original_currency = %v, want USD", dbPayment.ProviderPayload["original_currency"])
	}
	if dbPayment.ProviderPayload["exchange_rate"] != "7.2" {
		t.Errorf("exchange_rate = %v, want 7.2", dbPayment.ProviderPayload["exchange_rate"])
	}
}
