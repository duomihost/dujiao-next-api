package admin

import (
	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"

	"github.com/gin-gonic/gin"
)

// ====================  设置管理  ====================

// GetSettings 获取设置
func (h *Handler) GetSettings(c *gin.Context) {
	key := c.DefaultQuery("key", constants.SettingKeySiteConfig)

	value, err := h.SettingService.GetByKey(key)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	if value == nil {
		response.Success(c, gin.H{})
		return
	}

	response.Success(c, value)
}

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	Key   string                 `json:"key" binding:"required"`
	Value map[string]interface{} `json:"value" binding:"required"`
}

// UpdateSettings 更新设置
func (h *Handler) UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	value, err := h.SettingService.Update(req.Key, req.Value)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.settings_save_failed", err)
		return
	}

	if req.Key == constants.SettingKeySiteConfig {
		_ = cache.DelAllPublicConfig(c.Request.Context())
	}
	if req.Key == constants.SettingKeyRegistrationConfig {
		_ = cache.DelAllPublicConfig(c.Request.Context())
	}
	if req.Key == constants.SettingKeyNavConfig {
		_ = cache.DelAllPublicConfig(c.Request.Context())
	}
	if req.Key == constants.SettingKeyHomeAnnouncement {
		_ = cache.DelAllPublicConfig(c.Request.Context())
	}
	if req.Key == constants.SettingKeyWalletConfig {
		_ = cache.DelAllPublicConfig(c.Request.Context())
	}
	if req.Key == constants.SettingKeyCallbackRoutesConfig {
		h.SettingService.InvalidateCallbackRoutesCache()
	}
	response.Success(c, value)
}
