package admin

import (
	"errors"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// GetComplianceStatus GET /admin/compliance/status
func (h *Handler) GetComplianceStatus(c *gin.Context) {
	status, err := h.ComplianceService.Status()
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.internal", err)
		return
	}
	response.Success(c, status)
}

// ComplianceAcknowledgeRequest 请求体
type ComplianceAcknowledgeRequest struct {
	Segment1 string `json:"segment1" binding:"required"`
	Segment2 string `json:"segment2" binding:"required"`
	Segment3 string `json:"segment3" binding:"required"`
}

// AcknowledgeCompliance POST /admin/compliance/acknowledge —— 仅超管
func (h *Handler) AcknowledgeCompliance(c *gin.Context) {
	if !shared.IsSuperAdmin(c) {
		shared.RespondError(c, response.CodeForbidden, "compliance.error.super_admin_required", nil)
		return
	}
	adminID, ok := shared.GetAdminID(c)
	if !ok {
		return
	}
	username := ""
	if v, exists := c.Get("username"); exists {
		username, _ = v.(string)
	}

	var req ComplianceAcknowledgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	err := h.ComplianceService.Acknowledge(service.AcknowledgeRequest{
		Segment1:  req.Segment1,
		Segment2:  req.Segment2,
		Segment3:  req.Segment3,
		AdminID:   adminID,
		Username:  username,
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrComplianceTextMismatch):
			shared.RespondError(c, response.CodeBadRequest, "compliance.error.text_mismatch", nil)
			return
		case errors.Is(err, service.ErrAlreadyAcknowledged):
			response.Success(c, gin.H{"already_acknowledged": true})
			return
		default:
			shared.RespondError(c, response.CodeInternal, "error.internal", err)
			return
		}
	}
	response.Success(c, gin.H{"already_acknowledged": false})
}
