package admin

import (
	"errors"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// BannerUpsertRequest Banner 创建/更新请求
type BannerUpsertRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Position     string                 `json:"position"`
	TitleJSON    map[string]interface{} `json:"title"`
	SubtitleJSON map[string]interface{} `json:"subtitle"`
	Image        string                 `json:"image" binding:"required"`
	MobileImage  string                 `json:"mobile_image"`
	LinkType     string                 `json:"link_type"`
	LinkValue    string                 `json:"link_value"`
	OpenInNewTab *bool                  `json:"open_in_new_tab"`
	IsActive     *bool                  `json:"is_active"`
	StartAt      string                 `json:"start_at"`
	EndAt        string                 `json:"end_at"`
	SortOrder    int                    `json:"sort_order"`
}

func buildBannerInputFromRequest(req BannerUpsertRequest) (service.BannerInput, error) {
	startAt, err := shared.ParseTimeNullable(req.StartAt)
	if err != nil {
		return service.BannerInput{}, err
	}
	endAt, err := shared.ParseTimeNullable(req.EndAt)
	if err != nil {
		return service.BannerInput{}, err
	}
	return service.BannerInput{
		Name:         req.Name,
		Position:     req.Position,
		TitleJSON:    req.TitleJSON,
		SubtitleJSON: req.SubtitleJSON,
		Image:        req.Image,
		MobileImage:  req.MobileImage,
		LinkType:     req.LinkType,
		LinkValue:    req.LinkValue,
		OpenInNewTab: req.OpenInNewTab,
		IsActive:     req.IsActive,
		StartAt:      startAt,
		EndAt:        endAt,
		SortOrder:    req.SortOrder,
	}, nil
}

// GetAdminBanners 获取后台 Banner 列表
func (h *Handler) GetAdminBanners(c *gin.Context) {
	page, pageSize := shared.ParsePagination(c)

	position := c.Query("position")
	search := c.Query("search")

	isActive, err := shared.ParseQueryBoolPtr(c, "is_active")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	banners, total, err := h.BannerService.ListAdmin(position, search, isActive, page, pageSize)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.banner_fetch_failed", err)
		return
	}

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, banners, pagination)
}

// GetAdminBanner 获取后台 Banner 详情
func (h *Handler) GetAdminBanner(c *gin.Context) {
	id := c.Param("id")
	banner, err := h.BannerService.GetByID(id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			shared.RespondError(c, response.CodeNotFound, "error.banner_not_found", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.banner_fetch_failed", err)
		return
	}
	response.Success(c, banner)
}

// CreateBanner 创建 Banner
func (h *Handler) CreateBanner(c *gin.Context) {
	var req BannerUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	input, err := buildBannerInputFromRequest(req)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	banner, err := h.BannerService.Create(input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidBanner):
			shared.RespondError(c, response.CodeBadRequest, "error.banner_invalid", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.banner_create_failed", err)
		}
		return
	}

	response.Success(c, banner)
}

// UpdateBanner 更新 Banner
func (h *Handler) UpdateBanner(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	var req BannerUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	input, err := buildBannerInputFromRequest(req)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	banner, err := h.BannerService.Update(id, input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidBanner):
			shared.RespondError(c, response.CodeBadRequest, "error.banner_invalid", nil)
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.banner_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.banner_update_failed", err)
		}
		return
	}

	response.Success(c, banner)
}

// DeleteBanner 删除 Banner
func (h *Handler) DeleteBanner(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	if err := h.BannerService.Delete(id); err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			shared.RespondError(c, response.CodeNotFound, "error.banner_not_found", nil)
		default:
			shared.RespondError(c, response.CodeInternal, "error.banner_delete_failed", err)
		}
		return
	}

	response.Success(c, gin.H{
		"deleted": true,
	})
}
