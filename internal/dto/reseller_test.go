package dto

import (
	"testing"

	"github.com/dujiao-next/internal/models"
	"github.com/shopspring/decimal"
)

func TestResellerLedgerRespOmitsSensitiveSnapshotFields(t *testing.T) {
	orderID := uint(10)
	entry := models.ResellerLedgerEntry{
		ID:             1,
		ResellerID:     99,
		OrderID:        &orderID,
		Type:           models.ResellerLedgerTypeOrderProfit,
		Amount:         models.NewMoneyFromDecimal(decimal.RequireFromString("12.34")),
		Currency:       "USD",
		IdempotencyKey: "order_profit:10",
		MetadataJSON:   models.JSON{"pricing_snapshot_json": "hidden"},
		Status:         models.ResellerLedgerStatusAvailable,
	}

	resp := NewResellerLedgerResp(&entry)
	if resp.ID != 1 || resp.OrderID == nil || *resp.OrderID != 10 || resp.Amount != "12.34" {
		t.Fatalf("unexpected ledger response: %+v", resp)
	}
}

func TestResellerDashboardRespNotOpened(t *testing.T) {
	resp := NewResellerDashboardResp(false, nil, nil, false, "")
	if resp.Opened {
		t.Fatalf("expected unopened dashboard, got %+v", resp)
	}
	if resp.Profile != nil || len(resp.Balances) != 0 {
		t.Fatalf("unopened dashboard should not include profile or balances: %+v", resp)
	}
}

func TestResellerDashboardRespIncludesWithdrawAvailability(t *testing.T) {
	resp := NewResellerDashboardResp(true, &models.ResellerProfile{
		ID:               9,
		Status:           models.ResellerProfileStatusDisabled,
		SettlementStatus: models.ResellerSettlementStatusFrozen,
	}, nil, false, "settlement_unavailable")

	if !resp.Opened {
		t.Fatalf("expected opened dashboard, got %+v", resp)
	}
	if resp.WithdrawEnabled {
		t.Fatalf("expected withdraw disabled, got %+v", resp)
	}
	if resp.WithdrawDisabledReason != "settlement_unavailable" {
		t.Fatalf("unexpected withdraw disabled reason: %+v", resp)
	}
}

func TestResellerManagementSnapshotRespIncludesProfileAndDomains(t *testing.T) {
	profile := &models.ResellerProfile{
		ID:               12,
		Status:           models.ResellerProfileStatusPendingReview,
		ApplyReason:      "apply",
		SettlementStatus: models.ResellerSettlementStatusNormal,
	}
	domains := []models.ResellerDomain{{
		ID:                 33,
		ResellerID:         12,
		Domain:             "r12.shop.example.test",
		Type:               models.ResellerDomainTypeSubdomain,
		VerificationStatus: models.ResellerDomainVerificationVerified,
		Status:             models.ResellerDomainStatusActive,
		IsPrimary:          true,
	}}
	resp := NewResellerManagementSnapshotResp(profile, domains, false)
	if !resp.Opened || resp.Profile == nil || resp.Profile.ApplyReason != "apply" {
		t.Fatalf("unexpected snapshot profile response: %+v", resp)
	}
	if len(resp.Domains) != 1 || resp.Domains[0].Domain != "r12.shop.example.test" || !resp.Domains[0].IsPrimary {
		t.Fatalf("unexpected domains response: %+v", resp.Domains)
	}
}

func TestResellerDomainRespExposesVerificationTokenForOwner(t *testing.T) {
	row := &models.ResellerDomain{ID: 7, Domain: "shop.example.test", VerificationToken: "reseller-verify-token"}
	resp := NewResellerDomainResp(row)
	if resp.VerificationToken != "reseller-verify-token" {
		t.Fatalf("expected verification token exposed to owner/admin DTO, got %+v", resp)
	}
}

func TestResellerSiteConfigRespUsesSafeFields(t *testing.T) {
	row := &models.ResellerSiteConfig{
		ID:         10,
		ResellerID: 3,
		SiteName:   "Alice Store",
		Logo:       "/uploads/logo.png",
		Favicon:    "/uploads/favicon.png",
		SupportJSON: models.JSON{
			"telegram": "https://t.me/alice",
		},
		SEOJSON: models.JSON{
			"title": map[string]interface{}{"zh-CN": "标题"},
		},
		FooterLinksJSON: models.JSON{
			"items": []interface{}{map[string]interface{}{"url": "https://example.test"}},
		},
	}
	resp := NewResellerSiteConfigResp(row)
	if resp.ID != 10 || resp.SiteName != "Alice Store" || resp.Support["telegram"] != "https://t.me/alice" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.FooterLinks) != 1 {
		t.Fatalf("expected footer links unwrapped, got %+v", resp.FooterLinks)
	}
}
