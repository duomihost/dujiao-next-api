package service

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type ResellerManagementService struct {
	repo repository.ResellerRepository
	cfg  config.ResellerConfig
}

type ResellerApplyInput struct {
	Reason string
}

type ResellerApproveInput struct {
	DefaultMarkupPercent decimal.Decimal
	MaxMarkupPercent     decimal.Decimal
}

type ResellerApproveResult struct {
	Profile      *models.ResellerProfile
	SystemDomain *models.ResellerDomain
}

func NewResellerManagementService(repo repository.ResellerRepository, cfg config.ResellerConfig) *ResellerManagementService {
	return &ResellerManagementService{repo: repo, cfg: cfg}
}

func (s *ResellerManagementService) GetUserManagementSnapshot(userID uint) (*models.ResellerProfile, []models.ResellerDomain, bool, error) {
	if s == nil || s.repo == nil || userID == 0 {
		return nil, []models.ResellerDomain{}, false, nil
	}
	profile, err := s.repo.GetProfileByUserID(userID)
	if err != nil {
		return nil, nil, false, err
	}
	if profile == nil {
		return nil, []models.ResellerDomain{}, s.cfg.Enabled && s.cfg.SelfApplyEnabled, nil
	}
	domains, err := s.repo.ListDomainsByResellerID(profile.ID)
	if err != nil {
		return nil, nil, false, err
	}
	canApply := profile.Status == models.ResellerProfileStatusRejected && s.cfg.Enabled && s.cfg.SelfApplyEnabled
	return profile, domains, canApply, nil
}

func (s *ResellerManagementService) ApplyUserReseller(userID uint, input ResellerApplyInput) (*models.ResellerProfile, error) {
	if s == nil || s.repo == nil || userID == 0 {
		return nil, ErrNotFound
	}
	if !s.cfg.Enabled || !s.cfg.SelfApplyEnabled {
		return nil, ErrResellerApplyDisabled
	}
	reason := strings.TrimSpace(input.Reason)
	existing, err := s.repo.GetProfileByUserID(userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		profile := &models.ResellerProfile{
			UserID:           userID,
			Status:           models.ResellerProfileStatusPendingReview,
			ApplyReason:      reason,
			SettlementStatus: models.ResellerSettlementStatusNormal,
		}
		if err := s.repo.CreateProfile(profile); err != nil {
			return nil, err
		}
		return s.repo.GetProfileByID(profile.ID)
	}
	switch existing.Status {
	case models.ResellerProfileStatusRejected:
		existing.Status = models.ResellerProfileStatusPendingReview
		existing.ApplyReason = reason
		existing.RejectReason = ""
		existing.ReviewedBy = nil
		existing.ReviewedAt = nil
		if err := s.repo.UpdateProfile(existing); err != nil {
			return nil, err
		}
		return s.repo.GetProfileByID(existing.ID)
	case models.ResellerProfileStatusPendingReview, models.ResellerProfileStatusActive:
		return existing, nil
	case models.ResellerProfileStatusDisabled:
		return nil, ErrResellerProfileInactive
	default:
		return nil, ErrResellerProfileStatusInvalid
	}
}

func (s *ResellerManagementService) ApproveProfile(ctx context.Context, adminID, profileID uint, input ResellerApproveInput) (*ResellerApproveResult, error) {
	if s == nil || s.repo == nil || adminID == 0 || profileID == 0 {
		return nil, ErrNotFound
	}
	base := NormalizeResellerHost(s.cfg.SubdomainBase)
	if base == "" {
		return nil, ErrResellerSubdomainBaseMissing
	}
	var result *ResellerApproveResult
	err := s.repo.Transaction(func(tx *gorm.DB) error {
		repoTx := s.repo.WithTx(tx)
		profile, err := repoTx.GetProfileByID(profileID)
		if err != nil {
			return err
		}
		if profile == nil {
			return ErrNotFound
		}
		if profile.Status != models.ResellerProfileStatusPendingReview && profile.Status != models.ResellerProfileStatusRejected {
			return ErrResellerProfileStatusInvalid
		}
		now := time.Now()
		profile.Status = models.ResellerProfileStatusActive
		profile.RejectReason = ""
		profile.DefaultMarkupPercent = models.NewMoneyFromDecimal(input.DefaultMarkupPercent)
		profile.MaxMarkupPercent = models.NewMoneyFromDecimal(input.MaxMarkupPercent)
		profile.SettlementStatus = models.ResellerSettlementStatusNormal
		profile.ReviewedBy = &adminID
		profile.ReviewedAt = &now
		if err := repoTx.UpdateProfile(profile); err != nil {
			return err
		}
		systemDomain, err := repoTx.UpsertDomain(models.ResellerDomain{
			ResellerID:         profile.ID,
			Domain:             buildSystemResellerSubdomain(profile.ID, base),
			Type:               models.ResellerDomainTypeSubdomain,
			VerificationStatus: models.ResellerDomainVerificationVerified,
			Status:             models.ResellerDomainStatusActive,
			IsPrimary:          true,
			VerifiedAt:         &now,
		})
		if err != nil {
			return err
		}
		result = &ResellerApproveResult{Profile: profile, SystemDomain: systemDomain}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result != nil && result.SystemDomain != nil {
		_ = cache.DelResellerDomain(ctx, result.SystemDomain.Domain)
	}
	return result, nil
}

func (s *ResellerManagementService) RejectProfile(adminID, profileID uint, reason string) (*models.ResellerProfile, error) {
	return s.updateProfileReviewStatus(adminID, profileID, models.ResellerProfileStatusRejected, strings.TrimSpace(reason))
}

func (s *ResellerManagementService) DisableProfile(adminID, profileID uint, reason string) (*models.ResellerProfile, error) {
	return s.updateProfileReviewStatus(adminID, profileID, models.ResellerProfileStatusDisabled, strings.TrimSpace(reason))
}

func (s *ResellerManagementService) RestoreProfile(adminID, profileID uint) (*models.ResellerProfile, error) {
	return s.updateProfileReviewStatus(adminID, profileID, models.ResellerProfileStatusActive, "")
}

func (s *ResellerManagementService) SubmitUserCustomDomain(userID uint, rawDomain string) (*models.ResellerDomain, error) {
	if s == nil || s.repo == nil || userID == 0 {
		return nil, ErrNotFound
	}
	profile, err := s.repo.GetProfileByUserID(userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrResellerNotOpened
	}
	if profile.Status != models.ResellerProfileStatusActive {
		return nil, ErrResellerProfileInactive
	}
	domain, err := normalizeAndValidateCustomDomain(rawDomain, s.cfg)
	if err != nil {
		return nil, err
	}
	token, err := generateResellerDomainVerificationToken()
	if err != nil {
		return nil, err
	}
	row, err := s.repo.UpsertDomain(models.ResellerDomain{
		ResellerID:         profile.ID,
		Domain:             domain,
		Type:               models.ResellerDomainTypeCustom,
		VerificationToken:  token,
		VerificationStatus: models.ResellerDomainVerificationPending,
		Status:             models.ResellerDomainStatusPendingReview,
		IsPrimary:          false,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return nil, ErrResellerDomainConflict
		}
		return nil, err
	}
	return row, nil
}

func (s *ResellerManagementService) ApproveDomain(ctx context.Context, adminID, domainID uint) (*models.ResellerDomain, error) {
	return s.updateDomainStatus(ctx, adminID, domainID, models.ResellerDomainStatusActive)
}

func (s *ResellerManagementService) DisableDomain(ctx context.Context, adminID, domainID uint) (*models.ResellerDomain, error) {
	return s.updateDomainStatus(ctx, adminID, domainID, models.ResellerDomainStatusDisabled)
}

func (s *ResellerManagementService) updateProfileReviewStatus(adminID, profileID uint, targetStatus, reason string) (*models.ResellerProfile, error) {
	if s == nil || s.repo == nil || adminID == 0 || profileID == 0 {
		return nil, ErrNotFound
	}
	profile, err := s.repo.GetProfileByID(profileID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrNotFound
	}
	switch targetStatus {
	case models.ResellerProfileStatusRejected:
		if profile.Status != models.ResellerProfileStatusPendingReview {
			return nil, ErrResellerProfileStatusInvalid
		}
	case models.ResellerProfileStatusDisabled:
		if profile.Status != models.ResellerProfileStatusPendingReview && profile.Status != models.ResellerProfileStatusActive && profile.Status != models.ResellerProfileStatusRejected {
			return nil, ErrResellerProfileStatusInvalid
		}
	case models.ResellerProfileStatusActive:
		if profile.Status != models.ResellerProfileStatusDisabled {
			return nil, ErrResellerProfileStatusInvalid
		}
	default:
		return nil, ErrResellerProfileStatusInvalid
	}
	now := time.Now()
	profile.Status = targetStatus
	if targetStatus == models.ResellerProfileStatusRejected || targetStatus == models.ResellerProfileStatusDisabled {
		profile.RejectReason = strings.TrimSpace(reason)
	} else {
		profile.RejectReason = ""
	}
	profile.ReviewedBy = &adminID
	profile.ReviewedAt = &now
	if err := s.repo.UpdateProfile(profile); err != nil {
		return nil, err
	}
	return s.repo.GetProfileByID(profileID)
}

func (s *ResellerManagementService) updateDomainStatus(ctx context.Context, adminID, domainID uint, targetStatus string) (*models.ResellerDomain, error) {
	if s == nil || s.repo == nil || adminID == 0 || domainID == 0 {
		return nil, ErrNotFound
	}
	var domainForCache string
	err := s.repo.Transaction(func(tx *gorm.DB) error {
		repoTx := s.repo.WithTx(tx)
		domain, err := repoTx.GetDomainByIDForUpdate(domainID)
		if err != nil {
			return err
		}
		if domain == nil {
			return ErrNotFound
		}
		switch targetStatus {
		case models.ResellerDomainStatusActive:
			if domain.Status != models.ResellerDomainStatusPendingReview && domain.Status != models.ResellerDomainStatusDisabled {
				return ErrResellerDomainStatusInvalid
			}
			now := time.Now()
			domain.Status = models.ResellerDomainStatusActive
			domain.VerificationStatus = models.ResellerDomainVerificationVerified
			domain.VerifiedAt = &now
		case models.ResellerDomainStatusDisabled:
			if domain.Status != models.ResellerDomainStatusPendingReview && domain.Status != models.ResellerDomainStatusActive {
				return ErrResellerDomainStatusInvalid
			}
			domain.Status = models.ResellerDomainStatusDisabled
		default:
			return ErrResellerDomainStatusInvalid
		}
		if err := repoTx.UpdateDomain(domain); err != nil {
			return err
		}
		domainForCache = domain.Domain
		return nil
	})
	if err != nil {
		return nil, err
	}
	if domainForCache != "" {
		_ = cache.DelResellerDomain(ctx, domainForCache)
	}
	return s.repo.GetDomainByID(domainID)
}

func normalizeAndValidateCustomDomain(raw string, cfg config.ResellerConfig) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrResellerDomainInvalid
	}
	if strings.Contains(trimmed, "://") || strings.ContainsAny(trimmed, "/?#") {
		return "", ErrResellerDomainInvalid
	}
	domain := NormalizeResellerHost(trimmed)
	if domain == "" {
		return "", ErrResellerDomainInvalid
	}
	for _, mainHost := range cfg.MainHosts {
		if domain == NormalizeResellerHost(mainHost) {
			return "", ErrResellerDomainMainHostNotAllowed
		}
	}
	base := NormalizeResellerHost(cfg.SubdomainBase)
	if base != "" && (domain == base || strings.HasSuffix(domain, "."+base)) {
		return "", ErrResellerDomainMainHostNotAllowed
	}
	return domain, nil
}

func buildSystemResellerSubdomain(profileID uint, base string) string {
	return NormalizeResellerHost(fmt.Sprintf("r%d.%s", profileID, base))
}

func generateResellerDomainVerificationToken() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return "reseller-verify-" + strings.ToLower(token), nil
}
