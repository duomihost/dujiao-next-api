package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// UserAuthService 用户认证服务
type UserAuthService struct {
	cfg                   *config.Config
	userRepo              repository.UserRepository
	userOAuthIdentityRepo repository.UserOAuthIdentityRepository
	codeRepo              repository.EmailVerifyCodeRepository
	settingService        *SettingService
	emailService          *EmailService
	telegramAuthService   *TelegramAuthService
	memberLevelSvc        *MemberLevelService
}

// SetMemberLevelService 设置会员等级服务
func (s *UserAuthService) SetMemberLevelService(svc *MemberLevelService) {
	s.memberLevelSvc = svc
}

// NewUserAuthService 创建用户认证服务
func NewUserAuthService(
	cfg *config.Config,
	userRepo repository.UserRepository,
	userOAuthIdentityRepo repository.UserOAuthIdentityRepository,
	codeRepo repository.EmailVerifyCodeRepository,
	settingService *SettingService,
	emailService *EmailService,
	telegramAuthService *TelegramAuthService,
) *UserAuthService {
	return &UserAuthService{
		cfg:                   cfg,
		userRepo:              userRepo,
		userOAuthIdentityRepo: userOAuthIdentityRepo,
		codeRepo:              codeRepo,
		settingService:        settingService,
		emailService:          emailService,
		telegramAuthService:   telegramAuthService,
	}
}

// UserJWTClaims 用户 JWT 声明
type UserJWTClaims struct {
	UserID       uint   `json:"user_id"`
	Email        string `json:"email"`
	TokenVersion uint64 `json:"token_version"`
	Typ          string `json:"typ,omitempty"`
	jwt.RegisteredClaims
}

// UserChallengeClaims 用户 2FA 挑战 token claims
//
// 注：Typ 字段同时占用与 UserJWTClaims 兼容的 typ 键，写入 "2fa_challenge"，
// 防止挑战 token 在被错误地解析为 UserJWTClaims 时通过中间件的 typ 校验。
type UserChallengeClaims struct {
	UserID     uint   `json:"user_id"`
	JTI        string `json:"jti"`
	Purpose    string `json:"purpose"`
	RememberMe bool   `json:"remember_me"`
	Typ        string `json:"typ"`
	jwt.RegisteredClaims
}

// UserLoginResult 用户登录第一步结果
type UserLoginResult struct {
	RequiresTOTP       bool
	User               *models.User
	Token              string
	ExpiresAt          time.Time
	ChallengeToken     string
	ChallengeJTI       string
	ChallengeExpiresAt time.Time
}

const (
	// EmailChangeModeBindOnly 表示仅需校验新邮箱验证码（用于 Telegram 虚拟邮箱账号）
	EmailChangeModeBindOnly = "bind_only"
	// EmailChangeModeChangeWithOldAndNew 表示需要旧邮箱 + 新邮箱双验证码
	EmailChangeModeChangeWithOldAndNew = "change_with_old_and_new"
	// PasswordChangeModeSetWithoutOld 表示首次设置密码，不需要旧密码
	PasswordChangeModeSetWithoutOld = "set_without_old"
	// PasswordChangeModeChangeWithOld 表示修改密码，需要旧密码
	PasswordChangeModeChangeWithOld = "change_with_old"
)

// GenerateUserJWT 生成用户 JWT Token
func (s *UserAuthService) GenerateUserJWT(user *models.User, expireHours int) (string, time.Time, error) {
	resolvedHours := expireHours
	if resolvedHours <= 0 {
		resolvedHours = resolveUserJWTExpireHours(s.cfg.UserJWT)
	}
	expiresAt := time.Now().Add(time.Duration(resolvedHours) * time.Hour)
	claims := UserJWTClaims{
		UserID:       user.ID,
		Email:        user.Email,
		TokenVersion: user.TokenVersion,
		Typ:          TokenTypAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.UserJWT.SecretKey))
	if err != nil {
		return "", time.Time{}, err
	}
	return tokenString, expiresAt, nil
}

// ParseUserJWT 解析用户 JWT Token
func (s *UserAuthService) ParseUserJWT(tokenString string) (*UserJWTClaims, error) {
	parser := newHS256JWTParser()
	claims := &UserJWTClaims{}
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.UserJWT.SecretKey), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*UserJWTClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("无效的 token")
}

// SendVerifyCode 发送邮箱验证码
func (s *UserAuthService) SendVerifyCode(email, purpose, locale string) error {
	if s.emailService == nil {
		return ErrEmailServiceNotConfigured
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	if !isVerifyPurposeSupported(purpose) {
		return ErrInvalidVerifyPurpose
	}

	if purpose == constants.VerifyPurposeRegister {
		if err := s.checkRegistrationEmailDomain(normalized); err != nil {
			return err
		}
		exist, err := s.userRepo.GetByEmail(normalized)
		if err != nil {
			return err
		}
		if exist != nil {
			return ErrEmailExists
		}
	}

	if purpose == constants.VerifyPurposeReset {
		user, err := s.userRepo.GetByEmail(normalized)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNotFound
		}
		if strings.TrimSpace(user.Locale) != "" {
			locale = user.Locale
		}
	}

	if purpose == constants.VerifyPurposeTelegramBind {
		user, err := s.userRepo.GetByEmail(normalized)
		if err != nil {
			return err
		}
		if user == nil {
			return ErrNotFound
		}
		if strings.TrimSpace(user.Locale) != "" {
			locale = user.Locale
		}
	}

	return s.sendVerifyCode(normalized, purpose, locale)
}

func (s *UserAuthService) checkRegistrationEmailDomain(email string) error {
	if s == nil || s.settingService == nil {
		return nil
	}
	policy, err := s.settingService.GetRegistrationEmailDomainPolicy()
	if err != nil {
		return err
	}
	return CheckRegistrationEmailDomainAllowed(email, policy)
}

// Register 用户注册
func (s *UserAuthService) Register(email, password, code string, agreementAccepted bool, emailVerificationEnabled bool) (*models.User, string, time.Time, error) {
	if !agreementAccepted {
		return nil, "", time.Time{}, ErrAgreementRequired
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if err := s.checkRegistrationEmailDomain(normalized); err != nil {
		return nil, "", time.Time{}, err
	}
	if err := validatePassword(s.cfg.Security.PasswordPolicy, password); err != nil {
		return nil, "", time.Time{}, err
	}

	exist, err := s.userRepo.GetByEmail(normalized)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if exist != nil {
		return nil, "", time.Time{}, ErrEmailExists
	}

	if emailVerificationEnabled {
		if _, err := s.verifyCode(normalized, constants.VerifyPurposeRegister, code); err != nil {
			return nil, "", time.Time{}, err
		}
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	now := time.Now()
	nickname := resolveNicknameFromEmail(normalized)
	user := &models.User{
		Email:           normalized,
		PasswordHash:    string(hashedPassword),
		DisplayName:     nickname,
		Status:          constants.UserStatusActive,
		EmailVerifiedAt: &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, "", time.Time{}, err
	}

	token, expiresAt, err := s.GenerateUserJWT(user, 0)
	if err != nil {
		return nil, "", time.Time{}, err
	}

	user.LastLoginAt = &now
	if err := s.userRepo.Update(user); err != nil {
		return nil, "", time.Time{}, err
	}
	_ = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(user))

	// 分配默认会员等级（必须在最后一次 Update 之后，避免被覆盖）
	if s.memberLevelSvc != nil {
		_ = s.memberLevelSvc.AssignDefaultLevel(user.ID)
	}

	return user, token, expiresAt, nil
}

// LoginStep1 用户密码登录第一步：校验密码，根据是否启用 2FA 返回 challenge token 或正式 JWT。
func (s *UserAuthService) LoginStep1(email, password string, rememberMe bool) (*UserLoginResult, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByEmail(normalized)
	if err != nil {
		return nil, err
	}
	if user == nil {
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$dummyhashtopreventtimingattacksxxxxxxxxxxxxxxxxxx"), []byte(password))
		return nil, ErrInvalidCredentials
	}
	if strings.ToLower(user.Status) != constants.UserStatusActive {
		return nil, ErrUserDisabled
	}
	if user.EmailVerifiedAt == nil {
		return nil, ErrEmailNotVerified
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if user.TOTPEnabledAt != nil {
		challenge, jti, expiresAt, err := s.IssueUserChallengeToken(user.ID, rememberMe)
		if err != nil {
			return nil, err
		}
		return &UserLoginResult{
			RequiresTOTP:       true,
			User:               user,
			ChallengeToken:     challenge,
			ChallengeJTI:       jti,
			ChallengeExpiresAt: expiresAt,
		}, nil
	}

	expireHours := resolveUserJWTExpireHours(s.cfg.UserJWT)
	if rememberMe {
		expireHours = resolveRememberMeExpireHours(s.cfg.UserJWT)
	}
	token, expiresAt, err := s.GenerateUserJWT(user, expireHours)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	user.LastLoginAt = &now
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	_ = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(user))

	return &UserLoginResult{
		RequiresTOTP: false,
		User:         user,
		Token:        token,
		ExpiresAt:    expiresAt,
	}, nil
}

// IssueUserChallengeToken 签发用户 2FA 挑战 token
func (s *UserAuthService) IssueUserChallengeToken(userID uint, rememberMe bool) (token, jti string, expiresAt time.Time, err error) {
	jti = uuid.NewString()
	expiresAt = time.Now().Add(UserChallengeTTL)
	claims := UserChallengeClaims{
		UserID:     userID,
		JTI:        jti,
		Purpose:    UserChallengePurpose2FA,
		RememberMe: rememberMe,
		Typ:        TokenTyp2FAChallenge,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ID:        jti,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(s.cfg.UserJWT.SecretKey))
	if err != nil {
		return "", "", time.Time{}, err
	}
	return signed, jti, expiresAt, nil
}

// ParseUserChallengeToken 解析并校验用户挑战 token
func (s *UserAuthService) ParseUserChallengeToken(tokenString string) (*UserChallengeClaims, error) {
	parser := newHS256JWTParser()
	tok, err := parser.ParseWithClaims(tokenString, &UserChallengeClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.UserJWT.SecretKey), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*UserChallengeClaims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid challenge token")
	}
	if claims.Purpose != UserChallengePurpose2FA || claims.Typ != TokenTyp2FAChallenge {
		return nil, errors.New("invalid challenge purpose")
	}
	return claims, nil
}

// CompleteLoginAfter2FA 用户 2FA 验证通过后完成登录：发正式 JWT、更新 last_login
func (s *UserAuthService) CompleteLoginAfter2FA(userID uint, rememberMe bool) (*UserLoginResult, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	expireHours := resolveUserJWTExpireHours(s.cfg.UserJWT)
	if rememberMe {
		expireHours = resolveRememberMeExpireHours(s.cfg.UserJWT)
	}
	token, expiresAt, err := s.GenerateUserJWT(user, expireHours)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	user.LastLoginAt = &now
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	_ = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(user))
	return &UserLoginResult{RequiresTOTP: false, User: user, Token: token, ExpiresAt: expiresAt}, nil
}

func (s *UserAuthService) verifyCode(email, purpose, code string) (*models.EmailVerifyCode, error) {
	record, err := s.codeRepo.GetLatest(email, purpose)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrVerifyCodeInvalid
	}
	if record.VerifiedAt != nil {
		return nil, ErrVerifyCodeInvalid
	}

	now := time.Now()
	if record.ExpiresAt.Before(now) {
		return nil, ErrVerifyCodeExpired
	}

	maxAttempts := resolveMaxAttempts(s.cfg.Email.VerifyCode)
	if maxAttempts > 0 && record.AttemptCount >= maxAttempts {
		return nil, ErrVerifyCodeAttemptsExceeded
	}

	if strings.TrimSpace(record.Code) != strings.TrimSpace(code) {
		_ = s.codeRepo.IncrementAttempt(record.ID)
		return nil, ErrVerifyCodeInvalid
	}

	if err := s.codeRepo.MarkVerified(record.ID, now); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *UserAuthService) sendVerifyCode(email, purpose, locale string) error {
	latest, err := s.codeRepo.GetLatest(email, purpose)
	if err != nil {
		return err
	}
	now := time.Now()
	if latest != nil {
		interval := time.Duration(resolveSendIntervalSeconds(s.cfg.Email.VerifyCode)) * time.Second
		if !latest.SentAt.IsZero() && now.Sub(latest.SentAt) < interval {
			return ErrVerifyCodeTooFrequent
		}
	}

	code, err := randomNumericCode(resolveCodeLength(s.cfg.Email.VerifyCode))
	if err != nil {
		return err
	}

	record := &models.EmailVerifyCode{
		Email:     email,
		Purpose:   strings.ToLower(purpose),
		Code:      code,
		ExpiresAt: now.Add(time.Duration(resolveExpireMinutes(s.cfg.Email.VerifyCode)) * time.Minute),
		SentAt:    now,
		CreatedAt: now,
	}
	if err := s.emailService.SendVerifyCode(email, code, purpose, locale); err != nil {
		return err
	}

	if err := s.codeRepo.Create(record); err != nil {
		return err
	}

	return nil
}

func normalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return "", ErrInvalidEmail
	}
	if _, err := mail.ParseAddress(normalized); err != nil {
		return "", ErrInvalidEmail
	}
	return normalized, nil
}

// NormalizeEmail 统一邮箱格式
func NormalizeEmail(email string) (string, error) {
	return normalizeEmail(email)
}

func isVerifyPurposeSupported(purpose string) bool {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case constants.VerifyPurposeRegister, constants.VerifyPurposeReset, constants.VerifyPurposeTelegramBind, constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew:
		return true
	default:
		return false
	}
}

func resolveUserJWTExpireHours(cfg config.JWTConfig) int {
	if cfg.ExpireHours <= 0 {
		return 24
	}
	return cfg.ExpireHours
}

func resolveRememberMeExpireHours(cfg config.JWTConfig) int {
	if cfg.RememberMeExpireHours <= 0 {
		return resolveUserJWTExpireHours(cfg)
	}
	return cfg.RememberMeExpireHours
}

func resolveNicknameFromEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" {
		return strings.TrimSpace(parts[0])
	}
	return email
}

func resolveExpireMinutes(cfg config.VerifyCodeConfig) int {
	if cfg.ExpireMinutes <= 0 {
		return 10
	}
	return cfg.ExpireMinutes
}

func resolveSendIntervalSeconds(cfg config.VerifyCodeConfig) int {
	if cfg.SendIntervalSeconds <= 0 {
		return 60
	}
	return cfg.SendIntervalSeconds
}

func resolveMaxAttempts(cfg config.VerifyCodeConfig) int {
	if cfg.MaxAttempts <= 0 {
		return 5
	}
	return cfg.MaxAttempts
}

func resolveCodeLength(cfg config.VerifyCodeConfig) int {
	if cfg.Length < 4 || cfg.Length > 10 {
		return 6
	}
	return cfg.Length
}

func randomNumericCode(length int) (string, error) {
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteString(fmt.Sprintf("%d", n.Int64()))
	}
	return b.String(), nil
}
