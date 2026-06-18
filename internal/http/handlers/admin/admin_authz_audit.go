package admin

import (
	"strings"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/repository"

	"github.com/gin-gonic/gin"
)

// ListAuthzAuditLogs 获取权限审计日志列表
func (h *Handler) ListAuthzAuditLogs(c *gin.Context) {
	page, pageSize := shared.ParsePagination(c)

	operatorAdminIDRaw := c.Query("operator_admin_id")
	targetAdminIDRaw := c.Query("target_admin_id")
	action := strings.TrimSpace(c.Query("action"))
	role := strings.TrimSpace(c.Query("role"))
	object := strings.TrimSpace(c.Query("object"))
	method := strings.TrimSpace(c.Query("method"))

	var operatorAdminID uint
	if operatorAdminIDRaw != "" {
		parsedOperatorAdminID, err := shared.ParseQueryUint(operatorAdminIDRaw, false)
		if err != nil {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		operatorAdminID = parsedOperatorAdminID
	}

	var targetAdminID uint
	if targetAdminIDRaw != "" {
		parsedTargetAdminID, err := shared.ParseQueryUint(targetAdminIDRaw, false)
		if err != nil {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
		targetAdminID = parsedTargetAdminID
	}

	createdFrom, createdTo, err := shared.ParseQueryTimeRange(c, "created_from", "created_to")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	items, total, err := h.AuthzAuditService.ListForAdmin(repository.AuthzAuditLogListFilter{
		Page:            page,
		PageSize:        pageSize,
		OperatorAdminID: operatorAdminID,
		TargetAdminID:   targetAdminID,
		Action:          action,
		Role:            role,
		Object:          object,
		Method:          method,
		CreatedFrom:     createdFrom,
		CreatedTo:       createdTo,
	})
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.config_fetch_failed", err)
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, items, pagination)
}
