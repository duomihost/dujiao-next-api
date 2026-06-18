package public

import (
	"errors"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/dto"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

type telegramOIDCCallbackRequest struct {
	Code  string `json:"code" binding:"required"`
	State string `json:"state" binding:"required"`
}

func respondTelegramOIDCError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrTelegramAuthDisabled):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_auth_disabled", nil)
	case errors.Is(err, service.ErrTelegramAuthConfigInvalid):
		shared.RespondError(c, response.CodeInternal, "error.telegram_auth_config_invalid", err)
	case errors.Is(err, service.ErrTelegramOIDCStateInvalid):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_oidc_state_invalid", nil)
	case errors.Is(err, service.ErrTelegramOIDCTokenExchange):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_oidc_token_exchange_failed", err)
	case errors.Is(err, service.ErrTelegramOIDCIDTokenInvalid):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_oidc_id_token_invalid", nil)
	case errors.Is(err, service.ErrTelegramAuthPayloadInvalid):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_auth_payload_invalid", nil)
	case errors.Is(err, service.ErrTelegramAuthExpired):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_auth_expired", nil)
	case errors.Is(err, service.ErrTelegramAuthReplay):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_auth_replayed", nil)
	case errors.Is(err, service.ErrUserOAuthIdentityExists):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_bind_conflict", nil)
	case errors.Is(err, service.ErrUserOAuthAlreadyBound):
		shared.RespondError(c, response.CodeBadRequest, "error.telegram_already_bound", nil)
	case errors.Is(err, service.ErrUserDisabled):
		shared.RespondError(c, response.CodeUnauthorized, "error.user_disabled", nil)
	case errors.Is(err, service.ErrRegistrationDisabled):
		shared.RespondError(c, response.CodeForbidden, "error.registration_disabled", nil)
	default:
		shared.RespondError(c, response.CodeInternal, "error.login_failed", err)
	}
}

// StartTelegramOIDCLogin 返回 Telegram OIDC 授权 URL（登录流程）
func (h *Handler) StartTelegramOIDCLogin(c *gin.Context) {
	authURL, err := h.UserAuthService.StartTelegramOIDC(service.StartTelegramOIDCInput{
		Intent:  "login",
		Context: c.Request.Context(),
	})
	if err != nil {
		respondTelegramOIDCError(c, err)
		return
	}
	response.Success(c, gin.H{"auth_url": authURL})
}

// TelegramOIDCLoginCallback 处理 Telegram OIDC 回调（登录）
func (h *Handler) TelegramOIDCLoginCallback(c *gin.Context) {
	var req telegramOIDCCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.recordUserLogin(c, "", 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonBadRequest, constants.LoginLogSourceTelegram)
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	res, err := h.UserAuthService.LoginWithTelegramOIDC(service.LoginWithTelegramOIDCInput{
		Code:    req.Code,
		State:   req.State,
		Context: c.Request.Context(),
	})
	if err != nil {
		h.recordUserLogin(c, "", 0, constants.LoginLogStatusFailed, constants.LoginLogFailReasonTelegramInvalid, constants.LoginLogSourceTelegram)
		respondTelegramOIDCError(c, err)
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

// StartTelegramOIDCBind 返回 Telegram OIDC 授权 URL（绑定流程，需登录）
func (h *Handler) StartTelegramOIDCBind(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}
	authURL, err := h.UserAuthService.StartTelegramOIDC(service.StartTelegramOIDCInput{
		Intent:  "bind",
		UserID:  uid,
		Context: c.Request.Context(),
	})
	if err != nil {
		respondTelegramOIDCError(c, err)
		return
	}
	response.Success(c, gin.H{"auth_url": authURL})
}

// TelegramOIDCBindCallback 处理 Telegram OIDC 回调（绑定，需登录）
func (h *Handler) TelegramOIDCBindCallback(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}
	var req telegramOIDCCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	identity, err := h.UserAuthService.BindTelegramOIDC(service.BindTelegramOIDCInput{
		UserID:  uid,
		Code:    req.Code,
		State:   req.State,
		Context: c.Request.Context(),
	})
	if err != nil {
		respondTelegramOIDCError(c, err)
		return
	}
	response.Success(c, dto.NewTelegramBindingResp(identity))
}
