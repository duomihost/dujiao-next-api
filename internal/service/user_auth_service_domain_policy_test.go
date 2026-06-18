package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newRegistrationDomainPolicyAuthService(t *testing.T) (*UserAuthService, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:user_auth_domain_policy_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserOAuthIdentity{}, &models.EmailVerifyCode{}, &models.Setting{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	cfg := &config.Config{
		App:     config.AppConfig{SecretKey: "test-app-secret-domain-policy"},
		UserJWT: config.JWTConfig{SecretKey: "user-jwt-domain-policy-secret", ExpireHours: 24},
		Email:   config.EmailConfig{Enabled: false},
	}
	settingSvc := NewSettingService(repository.NewSettingRepository(db))
	return NewUserAuthService(
		cfg,
		repository.NewUserRepository(db),
		repository.NewUserOAuthIdentityRepository(db),
		repository.NewEmailVerifyCodeRepository(db),
		settingSvc,
		NewEmailService(&cfg.Email),
		nil,
	), db
}

func TestRegisterRejectsEmailDomainNotAllowed(t *testing.T) {
	svc, _ := newRegistrationDomainPolicyAuthService(t)
	if _, err := svc.settingService.Update(constants.SettingKeyRegistrationConfig, map[string]interface{}{
		constants.SettingFieldEmailDomainAllowlistEnabled: true,
		constants.SettingFieldAllowedEmailDomains:         []interface{}{"qq.com"},
	}); err != nil {
		t.Fatalf("update registration config failed: %v", err)
	}

	user, token, _, err := svc.Register("buyer@example.com", "secret123", "", true, false)
	if !errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Fatalf("expected ErrEmailDomainNotAllowed, got user=%+v token=%q err=%v", user, token, err)
	}
}

func TestRegisterAllowsExactEmailDomain(t *testing.T) {
	svc, _ := newRegistrationDomainPolicyAuthService(t)
	if _, err := svc.settingService.Update(constants.SettingKeyRegistrationConfig, map[string]interface{}{
		constants.SettingFieldEmailDomainAllowlistEnabled: true,
		constants.SettingFieldAllowedEmailDomains:         []interface{}{"qq.com"},
	}); err != nil {
		t.Fatalf("update registration config failed: %v", err)
	}

	user, token, _, err := svc.Register("buyer@qq.com", "secret123", "", true, false)
	if err != nil {
		t.Fatalf("register should allow qq.com: %v", err)
	}
	if user == nil || user.Email != "buyer@qq.com" || token == "" {
		t.Fatalf("unexpected register result user=%+v token=%q", user, token)
	}
}

func TestSendVerifyCodeRejectsEmailDomainBeforeEmailSend(t *testing.T) {
	svc, _ := newRegistrationDomainPolicyAuthService(t)
	if _, err := svc.settingService.Update(constants.SettingKeyRegistrationConfig, map[string]interface{}{
		constants.SettingFieldEmailDomainAllowlistEnabled: true,
		constants.SettingFieldAllowedEmailDomains:         []interface{}{"qq.com"},
	}); err != nil {
		t.Fatalf("update registration config failed: %v", err)
	}

	err := svc.SendVerifyCode("buyer@example.com", constants.VerifyPurposeRegister, constants.LocaleZhCN)
	if !errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Fatalf("expected ErrEmailDomainNotAllowed, got %v", err)
	}
}
