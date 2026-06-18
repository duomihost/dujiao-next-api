package public

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/dto"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// UserSendVerifyCodeRequest 发送验证码请求
type UserSendVerifyCodeRequest struct {
	Email          string                       `json:"email" binding:"required"`
	Purpose        string                       `json:"purpose" binding:"required"`
	CaptchaPayload shared.CaptchaPayloadRequest `json:"captcha_payload"`
}

// SendUserVerifyCode 发送用户邮箱验证码
func (h *Handler) SendUserVerifyCode(c *gin.Context) {
	var req UserSendVerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))

	// 检查邮箱验证开关（总开关）
	emailVerificationEnabled, err := h.SettingService.GetEmailVerificationEnabled(true)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.send_verify_code_failed", err)
		return
	}
	if !emailVerificationEnabled {
		shared.RespondError(c, response.CodeForbidden, "error.email_verification_disabled", nil)
		return
	}

	// 当 purpose 为 register 时，检查注册是否开启
	if purpose == constants.VerifyPurposeRegister {
		registrationEnabled, err := h.SettingService.GetRegistrationEnabled(true)
		if err != nil {
			shared.RespondError(c, response.CodeInternal, "error.send_verify_code_failed", err)
			return
		}
		if !registrationEnabled {
			shared.RespondError(c, response.CodeForbidden, "error.registration_disabled", nil)
			return
		}
	}

	captchaScene := ""
	switch purpose {
	case constants.VerifyPurposeRegister:
		captchaScene = constants.CaptchaSceneRegisterSendCode
	case constants.VerifyPurposeReset:
		captchaScene = constants.CaptchaSceneResetSendCode
	}
	if captchaScene != "" && h.CaptchaService != nil {
		if captchaErr := h.CaptchaService.Verify(captchaScene, req.CaptchaPayload.ToServicePayload(), c.ClientIP()); captchaErr != nil {
			switch {
			case errors.Is(captchaErr, service.ErrCaptchaRequired):
				shared.RespondError(c, response.CodeBadRequest, "error.captcha_required", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaInvalid):
				shared.RespondError(c, response.CodeBadRequest, "error.captcha_invalid", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaConfigInvalid):
				shared.RespondError(c, response.CodeInternal, "error.captcha_config_invalid", captchaErr)
				return
			default:
				shared.RespondError(c, response.CodeInternal, "error.captcha_verify_failed", captchaErr)
				return
			}
		}
	}

	locale := i18n.ResolveLocale(c)
	if err := h.UserAuthService.SendVerifyCode(req.Email, req.Purpose, locale); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			shared.RespondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrInvalidVerifyPurpose):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_purpose_invalid", nil)
		case errors.Is(err, service.ErrEmailExists):
			shared.RespondError(c, response.CodeBadRequest, "error.email_exists", nil)
		case errors.Is(err, service.ErrEmailDomainNotAllowed):
			shared.RespondError(c, response.CodeBadRequest, "error.email_domain_not_allowed", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		case errors.Is(err, service.ErrVerifyCodeTooFrequent):
			shared.RespondError(c, response.CodeTooManyRequests, "error.verify_code_too_frequent", nil)
		case errors.Is(err, service.ErrEmailRecipientRejected):
			shared.RespondError(c, response.CodeBadRequest, "error.email_recipient_not_found", nil)
		case errors.Is(err, service.ErrEmailServiceDisabled),
			errors.Is(err, service.ErrEmailServiceNotConfigured):
			shared.RespondError(c, response.CodeInternal, "error.email_service_not_configured", err)
		default:
			shared.RespondError(c, response.CodeInternal, "error.send_verify_code_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"sent": true})
}

// UserRegisterRequest 注册请求
type UserRegisterRequest struct {
	Email             string `json:"email" binding:"required"`
	Password          string `json:"password" binding:"required"`
	Code              string `json:"code"`
	AgreementAccepted bool   `json:"agreement_accepted"`
}

// UserRegister 用户注册
func (h *Handler) UserRegister(c *gin.Context) {
	var req UserRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	// 检查注册是否开启
	registrationEnabled, err := h.SettingService.GetRegistrationEnabled(true)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.register_failed", err)
		return
	}
	if !registrationEnabled {
		shared.RespondError(c, response.CodeForbidden, "error.registration_disabled", nil)
		return
	}

	// 检查邮箱验证开关
	emailVerificationEnabled, err := h.SettingService.GetEmailVerificationEnabled(true)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.register_failed", err)
		return
	}

	user, token, expiresAt, err := h.UserAuthService.Register(req.Email, req.Password, req.Code, req.AgreementAccepted, emailVerificationEnabled)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			shared.RespondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrEmailExists):
			shared.RespondError(c, response.CodeBadRequest, "error.email_exists", nil)
		case errors.Is(err, service.ErrEmailDomainNotAllowed):
			shared.RespondError(c, response.CodeBadRequest, "error.email_domain_not_allowed", nil)
		case errors.Is(err, service.ErrVerifyCodeInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_invalid", nil)
		case errors.Is(err, service.ErrVerifyCodeExpired):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_expired", nil)
		case errors.Is(err, service.ErrVerifyCodeAttemptsExceeded):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_attempts_exceeded", nil)
		case errors.Is(err, service.ErrAgreementRequired):
			shared.RespondError(c, response.CodeBadRequest, "error.agreement_required", nil)
		case errors.Is(err, service.ErrWeakPassword):
			locale := i18n.ResolveLocale(c)
			if perr, ok := err.(interface {
				Key() string
				Args() []interface{}
			}); ok {
				msg := i18n.Sprintf(locale, perr.Key(), perr.Args()...)
				shared.RespondErrorWithMsg(c, response.CodeBadRequest, msg, nil)
				return
			}
			shared.RespondError(c, response.CodeBadRequest, "error.password_weak", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.register_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"user":       dto.NewUserAuthBriefResp(user),
		"token":      token,
		"expires_at": expiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// UserLoginRequest 登录请求
type UserLoginRequest struct {
	Email          string                       `json:"email" binding:"required"`
	Password       string                       `json:"password" binding:"required"`
	RememberMe     bool                         `json:"remember_me"`
	CaptchaPayload shared.CaptchaPayloadRequest `json:"captcha_payload"`
}

// UserLogin 用户登录
func (h *Handler) UserLogin(c *gin.Context) {
	var req UserLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonBadRequest, constants.LoginLogSourceWeb)
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	if h.CaptchaService != nil {
		if captchaErr := h.CaptchaService.Verify(constants.CaptchaSceneLogin, req.CaptchaPayload.ToServicePayload(), c.ClientIP()); captchaErr != nil {
			switch {
			case errors.Is(captchaErr, service.ErrCaptchaRequired):
				h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonCaptchaRequired, constants.LoginLogSourceWeb)
				shared.RespondError(c, response.CodeBadRequest, "error.captcha_required", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaInvalid):
				h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonCaptchaInvalid, constants.LoginLogSourceWeb)
				shared.RespondError(c, response.CodeBadRequest, "error.captcha_invalid", nil)
				return
			case errors.Is(captchaErr, service.ErrCaptchaConfigInvalid):
				h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonCaptchaConfigInvalid, constants.LoginLogSourceWeb)
				shared.RespondError(c, response.CodeInternal, "error.captcha_config_invalid", captchaErr)
				return
			default:
				h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonCaptchaVerifyFailed, constants.LoginLogSourceWeb)
				shared.RespondError(c, response.CodeInternal, "error.captcha_verify_failed", captchaErr)
				return
			}
		}
	}

	res, err := h.UserAuthService.LoginStep1(req.Email, req.Password, req.RememberMe)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonInvalidEmail, constants.LoginLogSourceWeb)
			shared.RespondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrInvalidCredentials):
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonInvalidCredentials, constants.LoginLogSourceWeb)
			shared.RespondError(c, response.CodeUnauthorized, "error.login_invalid", nil)
		case errors.Is(err, service.ErrEmailNotVerified):
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonEmailNotVerified, constants.LoginLogSourceWeb)
			shared.RespondError(c, response.CodeUnauthorized, "error.email_not_verified", nil)
		case errors.Is(err, service.ErrUserDisabled):
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonUserDisabled, constants.LoginLogSourceWeb)
			shared.RespondError(c, response.CodeUnauthorized, "error.user_disabled", nil)
		default:
			h.recordUserLogin(c, req.Email, 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonInternalError, constants.LoginLogSourceWeb)
			shared.RespondError(c, response.CodeInternal, "error.login_failed", err)
		}
		return
	}

	if res.RequiresTOTP {
		h.recordUserLogin(c, res.User.Email, res.User.ID, constants.LoginLogStatusSuccess, constants.LoginLogPasswordOK2FAPending, constants.LoginLogSourceWeb)
		response.Success(c, gin.H{
			"requires_totp":        true,
			"challenge_token":      res.ChallengeToken,
			"challenge_expires_at": res.ChallengeExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		})
		return
	}

	h.recordUserLogin(c, res.User.Email, res.User.ID, constants.LoginLogStatusSuccess, "", constants.LoginLogSourceWeb)
	response.Success(c, gin.H{
		"requires_totp": false,
		"user":          dto.NewUserAuthBriefResp(res.User),
		"token":         res.Token,
		"expires_at":    res.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// UserTelegramLoginRequest Telegram 登录请求
type UserTelegramLoginRequest struct {
	ID        int64  `json:"id" binding:"required"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
	AuthDate  int64  `json:"auth_date" binding:"required"`
	Hash      string `json:"hash" binding:"required"`
}

// UserTelegramMiniAppAuthRequest Telegram Mini App 鉴权请求
type UserTelegramMiniAppAuthRequest struct {
	InitData      string `json:"init_data"`
	InitDataCamel string `json:"initData"`
}

// UserBindTelegramRequest 绑定 Telegram 请求
type UserBindTelegramRequest struct {
	ID        int64  `json:"id" binding:"required"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
	AuthDate  int64  `json:"auth_date" binding:"required"`
	Hash      string `json:"hash" binding:"required"`
}

func (r UserTelegramMiniAppAuthRequest) initData() string {
	if strings.TrimSpace(r.InitData) != "" {
		return strings.TrimSpace(r.InitData)
	}
	return strings.TrimSpace(r.InitDataCamel)
}

// UserTelegramLogin Telegram 登录
func (h *Handler) UserTelegramLogin(c *gin.Context) {
	var req UserTelegramLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.recordUserLogin(c, "", 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonBadRequest, constants.LoginLogSourceTelegram)
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	res, err := h.UserAuthService.LoginWithTelegram(service.LoginWithTelegramInput{
		Payload: service.TelegramLoginPayload{
			ID:        req.ID,
			FirstName: req.FirstName,
			LastName:  req.LastName,
			Username:  req.Username,
			PhotoURL:  req.PhotoURL,
			AuthDate:  req.AuthDate,
			Hash:      req.Hash,
		},
		Context: c.Request.Context(),
	})
	if err != nil {
		h.respondTelegramLoginError(c, err)
		return
	}

	if res.RequiresTOTP {
		h.recordUserLogin(c, res.User.Email, res.User.ID, constants.LoginLogStatusSuccess, constants.LoginLogPasswordOK2FAPending, constants.LoginLogSourceTelegram)
		response.Success(c, gin.H{
			"requires_totp":        true,
			"challenge_token":      res.ChallengeToken,
			"challenge_expires_at": res.ChallengeExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		})
		return
	}

	h.recordUserLogin(c, res.User.Email, res.User.ID, constants.LoginLogStatusSuccess, "", constants.LoginLogSourceTelegram)
	response.Success(c, gin.H{
		"requires_totp": false,
		"user":          dto.NewUserAuthBriefResp(res.User),
		"token":         res.Token,
		"expires_at":    res.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// UserTelegramMiniAppLogin Telegram Mini App 登录
func (h *Handler) UserTelegramMiniAppLogin(c *gin.Context) {
	var req UserTelegramMiniAppAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.initData() == "" {
		h.recordUserLogin(c, "", 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonBadRequest, constants.LoginLogSourceTelegram)
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	res, err := h.UserAuthService.LoginWithTelegramMiniApp(service.LoginWithTelegramMiniAppInput{
		InitData: req.initData(),
		Context:  c.Request.Context(),
	})
	if err != nil {
		h.respondTelegramLoginError(c, err)
		return
	}

	if res.RequiresTOTP {
		h.recordUserLogin(c, res.User.Email, res.User.ID, constants.LoginLogStatusSuccess, constants.LoginLogPasswordOK2FAPending, constants.LoginLogSourceTelegram)
		response.Success(c, gin.H{
			"requires_totp":        true,
			"challenge_token":      res.ChallengeToken,
			"challenge_expires_at": res.ChallengeExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		})
		return
	}

	h.recordUserLogin(c, res.User.Email, res.User.ID, constants.LoginLogStatusSuccess, "", constants.LoginLogSourceTelegram)
	response.Success(c, gin.H{
		"requires_totp": false,
		"user":          dto.NewUserAuthBriefResp(res.User),
		"token":         res.Token,
		"expires_at":    res.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *Handler) recordUserLogin(c *gin.Context, email string, userID uint, status, failReason, source string) {
	if h == nil || h.Container == nil || h.UserLoginLogService == nil {
		return
	}
	requestID := ""
	if c != nil {
		if rid, ok := c.Get("request_id"); ok {
			if value, ok := rid.(string); ok {
				requestID = strings.TrimSpace(value)
			}
		}
	}
	_ = h.UserLoginLogService.Record(service.RecordUserLoginInput{
		UserID:      userID,
		Email:       email,
		Status:      status,
		FailReason:  failReason,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
		LoginSource: source,
		RequestID:   requestID,
	})
}

// UserResetPasswordRequest 重置密码请求
type UserResetPasswordRequest struct {
	Email       string `json:"email" binding:"required"`
	Code        string `json:"code" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UserForgotPassword 忘记密码重置
func (h *Handler) UserForgotPassword(c *gin.Context) {
	// 邮箱验证关闭时，禁止密码重置，提示联系管理员
	emailVerificationEnabled, err := h.SettingService.GetEmailVerificationEnabled(true)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.reset_failed", err)
		return
	}
	if !emailVerificationEnabled {
		shared.RespondError(c, response.CodeForbidden, "error.password_reset_disabled", nil)
		return
	}

	var req UserResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	if err := h.UserAuthService.ResetPassword(req.Email, req.Code, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			shared.RespondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		case errors.Is(err, service.ErrVerifyCodeInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_invalid", nil)
		case errors.Is(err, service.ErrVerifyCodeExpired):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_expired", nil)
		case errors.Is(err, service.ErrVerifyCodeAttemptsExceeded):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_attempts_exceeded", nil)
		case errors.Is(err, service.ErrWeakPassword):
			locale := i18n.ResolveLocale(c)
			if perr, ok := err.(interface {
				Key() string
				Args() []interface{}
			}); ok {
				msg := i18n.Sprintf(locale, perr.Key(), perr.Args()...)
				shared.RespondErrorWithMsg(c, response.CodeBadRequest, msg, nil)
				return
			}
			shared.RespondError(c, response.CodeBadRequest, "error.password_weak", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.reset_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"reset": true})
}

// GetCurrentUser 获取当前用户信息
func (h *Handler) GetCurrentUser(c *gin.Context) {
	id, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	user, err := h.UserAuthService.GetUserByID(id)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	if user == nil {
		shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		return
	}

	profile, err := h.userProfileResponse(user)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.Success(c, profile)
}

// GetMyTelegramBinding 获取当前用户 Telegram 绑定
func (h *Handler) GetMyTelegramBinding(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}
	identity, err := h.UserAuthService.GetTelegramBinding(uid)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		}
		return
	}
	if identity == nil {
		response.Success(c, dto.NewTelegramBindingResp(nil))
		return
	}
	response.Success(c, dto.NewTelegramBindingResp(identity))
}

// BindMyTelegram 绑定当前用户 Telegram
func (h *Handler) BindMyTelegram(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}
	var req UserBindTelegramRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	identity, err := h.UserAuthService.BindTelegram(service.BindTelegramInput{
		UserID: uid,
		Payload: service.TelegramLoginPayload{
			ID:        req.ID,
			FirstName: req.FirstName,
			LastName:  req.LastName,
			Username:  req.Username,
			PhotoURL:  req.PhotoURL,
			AuthDate:  req.AuthDate,
			Hash:      req.Hash,
		},
		Context: c.Request.Context(),
	})
	if err != nil {
		respondTelegramBindError(c, err)
		return
	}
	response.Success(c, dto.NewTelegramBindingResp(identity))
}

// BindMyTelegramMiniApp 绑定当前用户的 Telegram Mini App 身份
func (h *Handler) BindMyTelegramMiniApp(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}
	var req UserTelegramMiniAppAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.initData() == "" {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	identity, err := h.UserAuthService.BindTelegramMiniApp(service.BindTelegramMiniAppInput{
		UserID:   uid,
		InitData: req.initData(),
		Context:  c.Request.Context(),
	})
	if err != nil {
		respondTelegramBindError(c, err)
		return
	}
	response.Success(c, dto.NewTelegramBindingResp(identity))
}

// UnbindMyTelegram 解绑当前用户 Telegram
func (h *Handler) UnbindMyTelegram(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}
	err := h.UserAuthService.UnbindTelegram(uid)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserOAuthNotBound):
			shared.RespondError(c, response.CodeBadRequest, "error.telegram_not_bound", nil)
		case errors.Is(err, service.ErrTelegramUnbindRequiresEmail):
			shared.RespondError(c, response.CodeBadRequest, "error.telegram_unbind_requires_email", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.user_update_failed", err)
		}
		return
	}
	response.Success(c, gin.H{"unbound": true})
}

func (h *Handler) userProfileResponse(user *models.User) (dto.UserProfileResp, error) {
	emailMode, err := h.UserAuthService.ResolveEmailChangeMode(user)
	if err != nil {
		return dto.UserProfileResp{}, err
	}
	passwordMode, err := h.UserAuthService.ResolvePasswordChangeMode(user)
	if err != nil {
		return dto.UserProfileResp{}, err
	}
	return dto.NewUserProfileResp(user, emailMode, passwordMode), nil
}

// UserProfileUpdateRequest 更新资料请求
type UserProfileUpdateRequest struct {
	Nickname *string `json:"nickname"`
	Locale   *string `json:"locale"`
}

// UpdateUserProfile 更新用户资料
func (h *Handler) UpdateUserProfile(c *gin.Context) {
	id, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	var req UserProfileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	user, err := h.UserAuthService.UpdateProfile(id, req.Nickname, req.Locale)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProfileEmpty):
			shared.RespondError(c, response.CodeBadRequest, "error.profile_empty", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.user_update_failed", err)
		}
		return
	}

	profile, err := h.userProfileResponse(user)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.user_update_failed", err)
		return
	}
	response.Success(c, profile)
}

// ChangeEmailSendCodeRequest 更换邮箱验证码请求
type ChangeEmailSendCodeRequest struct {
	Kind     string `json:"kind" binding:"required"`
	NewEmail string `json:"new_email"`
}

// SendChangeEmailCode 发送更换邮箱验证码
func (h *Handler) SendChangeEmailCode(c *gin.Context) {
	id, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	var req ChangeEmailSendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	locale := i18n.ResolveLocale(c)
	if err := h.UserAuthService.SendChangeEmailCode(id, req.Kind, req.NewEmail, locale); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			shared.RespondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrEmailChangeInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.email_change_invalid", nil)
		case errors.Is(err, service.ErrEmailChangeExists):
			shared.RespondError(c, response.CodeBadRequest, "error.email_change_exists", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		case errors.Is(err, service.ErrVerifyCodeTooFrequent):
			shared.RespondError(c, response.CodeTooManyRequests, "error.verify_code_too_frequent", nil)
		case errors.Is(err, service.ErrEmailRecipientRejected):
			shared.RespondError(c, response.CodeBadRequest, "error.email_recipient_not_found", nil)
		case errors.Is(err, service.ErrEmailServiceDisabled),
			errors.Is(err, service.ErrEmailServiceNotConfigured):
			shared.RespondError(c, response.CodeInternal, "error.email_service_not_configured", err)
		default:
			shared.RespondError(c, response.CodeInternal, "error.send_verify_code_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"sent": true})
}

// ChangeEmailRequest 更换邮箱请求
type ChangeEmailRequest struct {
	NewEmail string `json:"new_email" binding:"required"`
	OldCode  string `json:"old_code"`
	NewCode  string `json:"new_code" binding:"required"`
}

// ChangeEmail 更换邮箱
func (h *Handler) ChangeEmail(c *gin.Context) {
	id, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	var req ChangeEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	user, err := h.UserAuthService.ChangeEmail(id, req.NewEmail, req.OldCode, req.NewCode)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail):
			shared.RespondError(c, response.CodeBadRequest, "error.email_invalid", nil)
		case errors.Is(err, service.ErrEmailChangeInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.email_change_invalid", nil)
		case errors.Is(err, service.ErrEmailChangeExists):
			shared.RespondError(c, response.CodeBadRequest, "error.email_change_exists", nil)
		case errors.Is(err, service.ErrVerifyCodeInvalid):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_invalid", nil)
		case errors.Is(err, service.ErrVerifyCodeExpired):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_expired", nil)
		case errors.Is(err, service.ErrVerifyCodeAttemptsExceeded):
			shared.RespondError(c, response.CodeBadRequest, "error.verify_code_attempts_exceeded", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.email_change_failed", err)
		}
		return
	}

	profile, respErr := h.userProfileResponse(user)
	if respErr != nil {
		shared.RespondError(c, response.CodeInternal, "error.email_change_failed", respErr)
		return
	}
	response.Success(c, profile)
}

// ChangeUserPasswordRequest 用户改密请求
type ChangeUserPasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangeUserPassword 用户登录态修改密码
func (h *Handler) ChangeUserPassword(c *gin.Context) {
	id, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	var req ChangeUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	if err := h.UserAuthService.ChangePassword(id, req.OldPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidPassword):
			shared.RespondError(c, response.CodeBadRequest, "error.password_old_invalid", nil)
		case errors.Is(err, service.ErrWeakPassword):
			locale := i18n.ResolveLocale(c)
			if perr, ok := err.(interface {
				Key() string
				Args() []interface{}
			}); ok {
				msg := i18n.Sprintf(locale, perr.Key(), perr.Args()...)
				shared.RespondErrorWithMsg(c, response.CodeBadRequest, msg, nil)
				return
			}
			shared.RespondError(c, response.CodeBadRequest, "error.password_weak", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.save_failed", err)
		}
		return
	}

	response.Success(c, gin.H{"updated": true})
}
