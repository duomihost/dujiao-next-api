package public

import (
	"github.com/dujiao-next/internal/dto"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"

	"github.com/gin-gonic/gin"
)

// GetMyLoginLogs 获取当前用户登录日志
func (h *Handler) GetMyLoginLogs(c *gin.Context) {
	uid, ok := shared.GetUserID(c)
	if !ok {
		return
	}

	page, pageSize := shared.ParsePagination(c)

	logs, total, err := h.UserLoginLogService.ListByUser(uid, page, pageSize)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.user_login_log_fetch_failed", err)
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, dto.NewLoginLogRespList(logs), pagination)
}
