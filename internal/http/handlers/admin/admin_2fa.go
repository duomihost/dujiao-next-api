package admin

import (
	"context"
	"errors"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// Get2FAStatus 当前管理员 2FA 状态
func (h *Handler) Get2FAStatus(c *gin.Context) {
	adminID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	st, err := h.TOTPService.GetStatus(adminID)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.internal_error", err)
		return
	}
	response.Success(c, st)
}

// Setup2FA 开始绑定，返回 secret + otpauth url
func (h *Handler) Setup2FA(c *gin.Context) {
	adminID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	res, err := h.TOTPService.Setup(adminID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTOTPAlreadyEnabled):
			shared.RespondError(c, response.CodeBadRequest, "error.totp_already_enabled", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.internal_error", err)
		}
		return
	}
	h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FASetup, constants.AdminLoginStatusSuccess, "", nil)
	response.Success(c, res)
}

// Enable2FARequest 启用请求
type Enable2FARequest struct {
	Code string `json:"code" binding:"required"`
}

// Enable2FA 完成绑定
func (h *Handler) Enable2FA(c *gin.Context) {
	adminID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	var req Enable2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	res, err := h.TOTPService.Enable(adminID, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTOTPAlreadyEnabled):
			h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FAEnabled, constants.AdminLoginStatusFailed, constants.AdminLoginFailAlreadyEnabled, nil)
			shared.RespondError(c, response.CodeBadRequest, "error.totp_already_enabled", nil)
		case errors.Is(err, service.ErrTOTPPendingExpired):
			h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FAEnabled, constants.AdminLoginStatusFailed, constants.AdminLoginFailPendingExpired, nil)
			shared.RespondError(c, response.CodeBadRequest, "error.totp_pending_expired", nil)
		case errors.Is(err, service.ErrTOTPCodeInvalid):
			h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FAEnabled, constants.AdminLoginStatusFailed, constants.AdminLoginFailInvalidTOTPCode, nil)
			shared.RespondError(c, response.CodeBadRequest, "error.totp_code_invalid", nil)
		case errors.Is(err, service.ErrTOTPTooManyAttempts):
			h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FAEnabled, constants.AdminLoginStatusFailed, constants.AdminLoginFailTooManyAttempts, nil)
			shared.RespondError(c, response.CodeBadRequest, "error.totp_too_many_attempts", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.internal_error", err)
		}
		return
	}
	h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FAEnabled, constants.AdminLoginStatusSuccess, "", nil)
	response.Success(c, res)
}

// Disable2FARequest 关闭请求
type Disable2FARequest struct {
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

// Disable2FA 关闭 2FA
func (h *Handler) Disable2FA(c *gin.Context) {
	adminID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	var req Disable2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	if req.Code == "" && req.RecoveryCode == "" {
		shared.RespondError(c, response.CodeBadRequest, "error.totp_code_required", nil)
		return
	}
	isRecovery := req.RecoveryCode != ""
	codeArg := req.Code
	if isRecovery {
		codeArg = req.RecoveryCode
	}
	if err := h.TOTPService.Disable(adminID, codeArg, isRecovery); err != nil {
		switch {
		case errors.Is(err, service.ErrTOTPNotEnabled):
			shared.RespondError(c, response.CodeBadRequest, "error.totp_not_enabled", nil)
		case errors.Is(err, service.ErrTOTPCodeInvalid):
			h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FADisabled, constants.AdminLoginStatusFailed, constants.AdminLoginFailInvalidTOTPCode, nil)
			shared.RespondError(c, response.CodeBadRequest, "error.totp_code_invalid", nil)
		case errors.Is(err, service.ErrTOTPRecoveryInvalid):
			h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FADisabled, constants.AdminLoginStatusFailed, constants.AdminLoginFailInvalidRecoveryCode, nil)
			shared.RespondError(c, response.CodeBadRequest, "error.recovery_code_invalid", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.internal_error", err)
		}
		return
	}
	h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEvent2FADisabled, constants.AdminLoginStatusSuccess, "", nil)
	response.Success(c, nil)
}

// RegenerateRecoveryCodesRequest 重新生成恢复码请求
type RegenerateRecoveryCodesRequest struct {
	Code string `json:"code" binding:"required"`
}

// RegenerateRecoveryCodes 重新生成恢复码
func (h *Handler) RegenerateRecoveryCodes(c *gin.Context) {
	adminID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	var req RegenerateRecoveryCodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	codes, err := h.TOTPService.RegenerateRecoveryCodes(adminID, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTOTPNotEnabled):
			shared.RespondError(c, response.CodeBadRequest, "error.totp_not_enabled", nil)
		case errors.Is(err, service.ErrTOTPCodeInvalid):
			h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEventRecoveryRegenerated, constants.AdminLoginStatusFailed, constants.AdminLoginFailInvalidTOTPCode, nil)
			shared.RespondError(c, response.CodeBadRequest, "error.totp_code_invalid", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.internal_error", err)
		}
		return
	}
	h.writeAdminLoginLog(c, adminID, getAdminUsername(c), constants.AdminLoginEventRecoveryRegenerated, constants.AdminLoginStatusSuccess, "", nil)
	response.Success(c, gin.H{"recovery_codes": codes})
}

// Verify2FARequest 第二步登录请求
type Verify2FARequest struct {
	ChallengeToken string `json:"challenge_token" binding:"required"`
	Code           string `json:"code"`
	RecoveryCode   string `json:"recovery_code"`
}

// Verify2FA 完成两步登录
func (h *Handler) Verify2FA(c *gin.Context) {
	var req Verify2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	if req.Code == "" && req.RecoveryCode == "" {
		shared.RespondError(c, response.CodeBadRequest, "error.totp_code_required", nil)
		return
	}
	claims, err := h.AuthService.ParseChallengeToken(req.ChallengeToken)
	if err != nil {
		shared.RespondError(c, response.CodeUnauthorized, "error.totp_challenge_invalid", nil)
		return
	}
	ctx := context.Background()
	rdb := cache.Client()
	if rdb != nil {
		if v, _ := rdb.Exists(ctx, service.ChallengeRevokedKey(claims.JTI)).Result(); v == 1 {
			shared.RespondError(c, response.CodeUnauthorized, "error.totp_challenge_invalid", nil)
			return
		}
	}
	verifyErr := h.verifyChallengeAttempt(claims.AdminID, req.Code, req.RecoveryCode)
	username := ""
	if a, _ := h.AuthService.AdminRepo().GetByID(claims.AdminID); a != nil {
		username = a.Username
	}
	if verifyErr != nil {
		failCnt := h.bumpChallengeFails(ctx, claims.JTI)
		event := constants.AdminLoginEventLogin2FAVerify
		failReason := constants.AdminLoginFailInvalidTOTPCode
		if req.RecoveryCode != "" {
			event = constants.AdminLoginEventLoginRecoveryCode
			failReason = constants.AdminLoginFailInvalidRecoveryCode
		}
		h.writeAdminLoginLog(c, claims.AdminID, username, event, constants.AdminLoginStatusFailed, failReason, nil)
		if failCnt >= service.ChallengeMaxFailures {
			h.revokeChallenge(ctx, claims.JTI)
			shared.RespondError(c, response.CodeUnauthorized, "error.totp_too_many_attempts", nil)
			return
		}
		switch {
		case errors.Is(verifyErr, service.ErrTOTPCodeInvalid):
			shared.RespondError(c, response.CodeUnauthorized, "error.totp_code_invalid", nil)
		case errors.Is(verifyErr, service.ErrTOTPRecoveryInvalid):
			shared.RespondError(c, response.CodeUnauthorized, "error.recovery_code_invalid", nil)
		default:
			shared.RespondError(c, response.CodeUnauthorized, "error.totp_challenge_invalid", nil)
		}
		return
	}
	h.revokeChallenge(ctx, claims.JTI)
	loginRes, err := h.AuthService.CompleteLoginAfter2FA(claims.AdminID)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.login_failed", err)
		return
	}
	successEvent := constants.AdminLoginEventLogin2FAVerify
	if req.RecoveryCode != "" {
		successEvent = constants.AdminLoginEventLoginRecoveryCode
	}
	h.writeAdminLoginLog(c, claims.AdminID, username, successEvent, constants.AdminLoginStatusSuccess, "", nil)
	response.Success(c, gin.H{
		"requires_totp": false,
		"token":         loginRes.Token,
		"user":          gin.H{"id": loginRes.Admin.ID, "username": loginRes.Admin.Username},
		"expires_at":    loginRes.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *Handler) verifyChallengeAttempt(adminID uint, code, recoveryCode string) error {
	if recoveryCode != "" {
		return h.TOTPService.VerifyChallengeRecoveryCode(adminID, recoveryCode)
	}
	return h.TOTPService.VerifyChallengeCode(adminID, code)
}

func (h *Handler) bumpChallengeFails(ctx context.Context, jti string) int64 {
	rdb := cache.Client()
	if rdb == nil {
		return 0
	}
	cnt, err := rdb.Incr(ctx, service.ChallengeFailKey(jti)).Result()
	if err == nil && cnt == 1 {
		_ = rdb.Expire(ctx, service.ChallengeFailKey(jti), service.ChallengeTTL).Err()
	}
	return cnt
}

func (h *Handler) revokeChallenge(ctx context.Context, jti string) {
	rdb := cache.Client()
	if rdb == nil {
		return
	}
	_ = rdb.Set(ctx, service.ChallengeRevokedKey(jti), "1", service.ChallengeTTL).Err()
}

// ResetTargetAdmin2FA 超管重置某管理员 2FA
func (h *Handler) ResetTargetAdmin2FA(c *gin.Context) {
	operatorID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	if !shared.IsSuperAdmin(c) {
		shared.RespondError(c, response.CodeForbidden, "error.forbidden", nil)
		return
	}
	targetID, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	if err := h.TOTPService.AdminReset(operatorID, targetID); err != nil {
		switch {
		case errors.Is(err, service.ErrTOTPCannotResetSelf):
			shared.RespondError(c, response.CodeBadRequest, "error.totp_cannot_reset_self", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.user_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.internal_error", err)
		}
		return
	}
	target, _ := h.AuthService.AdminRepo().GetByID(targetID)
	username := ""
	if target != nil {
		username = target.Username
	}
	op := operatorID
	h.writeAdminLoginLog(c, targetID, username, constants.AdminLoginEvent2FAResetByAdmin, constants.AdminLoginStatusSuccess, "", &op)
	response.Success(c, nil)
}

// ---- 辅助 ----

func (h *Handler) writeAdminLoginLog(c *gin.Context, adminID uint, username, eventType, status, failReason string, operatorID *uint) {
	if h.AdminLoginLogRepo == nil {
		return
	}
	requestID, _ := c.Get("request_id")
	rid, _ := requestID.(string)
	log := &models.AdminLoginLog{
		AdminID:    adminID,
		Username:   username,
		EventType:  eventType,
		Status:     status,
		FailReason: failReason,
		ClientIP:   c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		RequestID:  rid,
		OperatorID: operatorID,
	}
	_ = h.AdminLoginLogRepo.Create(log)
}

func getAdminUsername(c *gin.Context) string {
	return c.GetString("username")
}
