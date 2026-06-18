package admin

import (
	"strconv"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"

	"github.com/gin-gonic/gin"
)

// ====================  素材管理  ====================

type BatchDeleteMediaRequest struct {
	IDs []uint `json:"ids" binding:"required,min=1"`
}

// GetAdminMedia 素材列表
func (h *Handler) GetAdminMedia(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	scene := c.Query("scene")
	search := c.Query("search")

	items, total, err := h.MediaService.List(scene, search, page, pageSize)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.internal", err)
		return
	}

	response.Success(c, gin.H{
		"items": items,
		"total": total,
	})
}

// UpdateMedia 更新素材信息（重命名）
func (h *Handler) UpdateMedia(c *gin.Context) {
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.invalid_id", nil)
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.invalid_params", nil)
		return
	}

	if err := h.MediaService.Rename(id, req.Name); err != nil {
		shared.RespondError(c, response.CodeInternal, "error.internal", err)
		return
	}

	response.Success(c, nil)
}

// BatchDeleteMedia 批量删除素材
func (h *Handler) BatchDeleteMedia(c *gin.Context) {
	var req BatchDeleteMediaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	successCount, failedIDs := h.MediaService.BatchDelete(req.IDs)
	response.Success(c, gin.H{
		"total":         len(req.IDs),
		"success_count": successCount,
		"failed_ids":    failedIDs,
	})
}

// DeleteMedia 删除素材
func (h *Handler) DeleteMedia(c *gin.Context) {
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.invalid_id", nil)
		return
	}

	if err := h.MediaService.Delete(id); err != nil {
		shared.RespondError(c, response.CodeInternal, "error.internal", err)
		return
	}

	response.Success(c, nil)
}
