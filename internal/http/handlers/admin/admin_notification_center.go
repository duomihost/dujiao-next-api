package admin

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// GetNotificationCenterSettings 获取通知中心配置
func (h *Handler) GetNotificationCenterSettings(c *gin.Context) {
	setting, err := h.SettingService.GetNotificationCenterSetting()
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	response.Success(c, service.MaskNotificationCenterSettingForAdmin(setting))
}

// UpdateNotificationCenterSettings 更新通知中心配置
func (h *Handler) UpdateNotificationCenterSettings(c *gin.Context) {
	var req service.NotificationCenterSettingPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	setting, err := h.SettingService.PatchNotificationCenterSetting(req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotificationConfigInvalid):
			shared.RespondErrorWithMsg(c, response.CodeBadRequest, err.Error(), nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.settings_save_failed", err)
		}
		return
	}
	response.Success(c, service.MaskNotificationCenterSettingForAdmin(setting))
}

// NotificationCenterTestSendRequest 通知中心测试发送请求
type NotificationCenterTestSendRequest struct {
	Channel   string                 `json:"channel" binding:"required"`
	Target    string                 `json:"target" binding:"required"`
	Scene     string                 `json:"scene"`
	Locale    string                 `json:"locale"`
	Variables map[string]interface{} `json:"variables"`
}

// ListNotificationLogs 获取通知发送日志列表
func (h *Handler) ListNotificationLogs(c *gin.Context) {
	page, pageSize := shared.ParsePagination(c)

	channel := strings.ToLower(strings.TrimSpace(c.Query("channel")))
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	eventType := strings.ToLower(strings.TrimSpace(c.Query("event_type")))

	isTest, err := shared.ParseQueryBoolPtr(c, "is_test")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	createdFrom, createdTo, err := shared.ParseQueryTimeRange(c, "created_from", "created_to")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	items, total, err := h.NotificationLogService.ListForAdmin(repository.NotificationLogListFilter{
		Page:        page,
		PageSize:    pageSize,
		Channel:     channel,
		Status:      status,
		EventType:   eventType,
		IsTest:      isTest,
		CreatedFrom: createdFrom,
		CreatedTo:   createdTo,
	})
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}

	response.SuccessWithPage(c, items, response.BuildPagination(page, pageSize, total))
}

// TestNotificationCenterSettings 通知中心测试发送
func (h *Handler) TestNotificationCenterSettings(c *gin.Context) {
	var req NotificationCenterTestSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if channel != "email" && channel != "telegram" {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	if h.NotificationService == nil {
		shared.RespondError(c, response.CodeInternal, "error.notification_send_failed", nil)
		return
	}

	err := h.NotificationService.SendTest(c.Request.Context(), service.NotificationTestSendInput{
		Channel:   channel,
		Target:    strings.TrimSpace(req.Target),
		Scene:     strings.TrimSpace(req.Scene),
		Locale:    strings.TrimSpace(req.Locale),
		Variables: req.Variables,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotificationConfigInvalid):
			shared.RespondErrorWithMsg(c, response.CodeBadRequest, err.Error(), nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.notification_send_failed", err)
		}
		return
	}
	response.Success(c, gin.H{"sent": true})
}
