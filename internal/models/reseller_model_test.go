package models

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func openResellerModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:reseller_model_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Order{}); err != nil {
		t.Fatalf("migrate base models failed: %v", err)
	}
	if err := db.AutoMigrate(
		&ResellerProfile{},
		&ResellerDomain{},
		&ResellerSiteConfig{},
		&ResellerProductSetting{},
		&ResellerOrderSnapshot{},
		&ResellerLedgerEntry{},
		&ResellerWithdrawRequest{},
		&ResellerBalanceAccount{},
		&ResellerRelatedAccount{},
	); err != nil {
		t.Fatalf("migrate reseller models failed: %v", err)
	}
	if err := ensureResellerIndexes(db); err != nil {
		t.Fatalf("ensure reseller indexes failed: %v", err)
	}
	return db
}

func TestResellerModelsAutoMigrateAndOrderColumns(t *testing.T) {
	db := openResellerModelTestDB(t)
	if !db.Migrator().HasTable(&ResellerProfile{}) {
		t.Fatal("expected reseller_profiles table")
	}
	if !db.Migrator().HasTable(&ResellerDomain{}) {
		t.Fatal("expected reseller_domains table")
	}
	if !db.Migrator().HasColumn(&Order{}, "reseller_id") {
		t.Fatal("expected orders.reseller_id column")
	}
	if !db.Migrator().HasColumn(&Order{}, "reseller_domain") {
		t.Fatal("expected orders.reseller_domain column")
	}
	if !db.Migrator().HasColumn(&Order{}, "reseller_profit_amount") {
		t.Fatal("expected orders.reseller_profit_amount column")
	}
}

func TestResellerDomainActiveUniqueAllowsSoftDeleteRecreate(t *testing.T) {
	db := openResellerModelTestDB(t)
	user := User{Email: "reseller@example.com", PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	profile := ResellerProfile{UserID: user.ID, Status: ResellerProfileStatusActive, SettlementStatus: ResellerSettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	first := ResellerDomain{
		ResellerID:         profile.ID,
		Domain:             "shop.example.test",
		Type:               ResellerDomainTypeCustom,
		VerificationStatus: ResellerDomainVerificationVerified,
		Status:             ResellerDomainStatusActive,
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first domain failed: %v", err)
	}
	if err := db.Delete(&first).Error; err != nil {
		t.Fatalf("soft delete domain failed: %v", err)
	}
	second := ResellerDomain{
		ResellerID:         profile.ID,
		Domain:             "shop.example.test",
		Type:               ResellerDomainTypeCustom,
		VerificationStatus: ResellerDomainVerificationVerified,
		Status:             ResellerDomainStatusActive,
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("create second domain after soft delete failed: %v", err)
	}
}

func TestResellerMoneyFieldsRoundTrip(t *testing.T) {
	db := openResellerModelTestDB(t)
	amount := NewMoneyFromDecimal(decimal.RequireFromString("12.345"))
	entry := ResellerLedgerEntry{
		ResellerID:     1,
		Type:           ResellerLedgerTypeManualAdjust,
		Amount:         amount,
		Currency:       "CNY",
		IdempotencyKey: "manual:test:1",
		Status:         ResellerLedgerStatusAvailable,
	}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatalf("create ledger failed: %v", err)
	}
	var got ResellerLedgerEntry
	if err := db.First(&got, entry.ID).Error; err != nil {
		t.Fatalf("load ledger failed: %v", err)
	}
	if got.Amount.String() != "12.35" {
		t.Fatalf("amount should round to 12.35, got %s", got.Amount.String())
	}
}
