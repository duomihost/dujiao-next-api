package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func openResellerManagementServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:reseller_management_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Admin{}, &models.ResellerProfile{}, &models.ResellerDomain{}, &models.ResellerSiteConfig{}); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}

func seedResellerManagementUser(t *testing.T, db *gorm.DB, email string) models.User {
	t.Helper()
	user := models.User{Email: email, PasswordHash: "hash", DisplayName: email}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	return user
}

func newResellerManagementServiceForTest(db *gorm.DB) *ResellerManagementService {
	return NewResellerManagementService(repository.NewResellerRepository(db), config.ResellerConfig{
		Enabled:          true,
		SelfApplyEnabled: true,
		SubdomainBase:    "shop.example.test",
		MainHosts:        []string{"main.example.test"},
	})
}

func TestResellerManagementApplyCreatesPendingAndReapplyRejected(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	user := seedResellerManagementUser(t, db, "apply-reseller@example.test")
	svc := newResellerManagementServiceForTest(db)

	profile, err := svc.ApplyUserReseller(user.ID, ResellerApplyInput{Reason: "first application"})
	if err != nil {
		t.Fatalf("ApplyUserReseller failed: %v", err)
	}
	if profile.Status != models.ResellerProfileStatusPendingReview || profile.ApplyReason != "first application" {
		t.Fatalf("unexpected created profile: %+v", profile)
	}

	profile.Status = models.ResellerProfileStatusRejected
	profile.RejectReason = "missing info"
	reviewer := uint(7)
	now := time.Now()
	profile.ReviewedBy = &reviewer
	profile.ReviewedAt = &now
	if err := db.Save(profile).Error; err != nil {
		t.Fatalf("save rejected profile failed: %v", err)
	}

	reapplied, err := svc.ApplyUserReseller(user.ID, ResellerApplyInput{Reason: "second application"})
	if err != nil {
		t.Fatalf("reapply failed: %v", err)
	}
	if reapplied.Status != models.ResellerProfileStatusPendingReview || reapplied.ApplyReason != "second application" || reapplied.RejectReason != "" || reapplied.ReviewedBy != nil || reapplied.ReviewedAt != nil {
		t.Fatalf("unexpected reapplied profile: %+v", reapplied)
	}
}

func TestResellerManagementApplyDisabledConfigRejects(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	user := seedResellerManagementUser(t, db, "apply-disabled@example.test")
	svc := NewResellerManagementService(repository.NewResellerRepository(db), config.ResellerConfig{Enabled: false, SelfApplyEnabled: true})

	_, err := svc.ApplyUserReseller(user.ID, ResellerApplyInput{Reason: "want access"})
	if !errors.Is(err, ErrResellerApplyDisabled) {
		t.Fatalf("expected ErrResellerApplyDisabled, got %v", err)
	}
}

func TestResellerManagementApproveCreatesSystemSubdomain(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	user := seedResellerManagementUser(t, db, "approve-reseller@example.test")
	svc := newResellerManagementServiceForTest(db)
	profile, err := svc.ApplyUserReseller(user.ID, ResellerApplyInput{Reason: "approve me"})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	approved, err := svc.ApproveProfile(context.Background(), 9, profile.ID, ResellerApproveInput{
		DefaultMarkupPercent: decimal.NewFromInt(12),
		MaxMarkupPercent:     decimal.NewFromInt(45),
	})
	if err != nil {
		t.Fatalf("ApproveProfile failed: %v", err)
	}
	if approved.Profile.Status != models.ResellerProfileStatusActive || approved.Profile.ReviewedBy == nil || *approved.Profile.ReviewedBy != 9 {
		t.Fatalf("unexpected approved profile: %+v", approved.Profile)
	}
	if approved.SystemDomain == nil || approved.SystemDomain.Domain != fmt.Sprintf("r%d.shop.example.test", profile.ID) || approved.SystemDomain.Status != models.ResellerDomainStatusActive || approved.SystemDomain.VerificationStatus != models.ResellerDomainVerificationVerified || !approved.SystemDomain.IsPrimary {
		t.Fatalf("unexpected system domain: %+v", approved.SystemDomain)
	}
}

func TestResellerManagementSubmitCustomDomainRequiresActiveProfile(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	user := seedResellerManagementUser(t, db, "custom-domain@example.test")
	svc := newResellerManagementServiceForTest(db)
	if _, err := svc.ApplyUserReseller(user.ID, ResellerApplyInput{Reason: "pending"}); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	_, err := svc.SubmitUserCustomDomain(user.ID, "shop.customer.example")
	if !errors.Is(err, ErrResellerProfileInactive) {
		t.Fatalf("expected ErrResellerProfileInactive, got %v", err)
	}
}

func TestResellerManagementSubmitAndApproveCustomDomain(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	user := seedResellerManagementUser(t, db, "domain-approve@example.test")
	svc := newResellerManagementServiceForTest(db)
	profile, err := svc.ApplyUserReseller(user.ID, ResellerApplyInput{Reason: "approve me"})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if _, err := svc.ApproveProfile(context.Background(), 9, profile.ID, ResellerApproveInput{}); err != nil {
		t.Fatalf("approve profile failed: %v", err)
	}

	domain, err := svc.SubmitUserCustomDomain(user.ID, "Shop.Customer.Example:443")
	if err != nil {
		t.Fatalf("SubmitUserCustomDomain failed: %v", err)
	}
	if domain.Domain != "shop.customer.example" || domain.Type != models.ResellerDomainTypeCustom || domain.VerificationStatus != models.ResellerDomainVerificationPending || domain.Status != models.ResellerDomainStatusPendingReview || domain.VerificationToken == "" {
		t.Fatalf("unexpected submitted domain: %+v", domain)
	}

	approved, err := svc.ApproveDomain(context.Background(), 9, domain.ID)
	if err != nil {
		t.Fatalf("ApproveDomain failed: %v", err)
	}
	if approved.Status != models.ResellerDomainStatusActive || approved.VerificationStatus != models.ResellerDomainVerificationVerified || approved.VerifiedAt == nil {
		t.Fatalf("unexpected approved domain: %+v", approved)
	}
}
