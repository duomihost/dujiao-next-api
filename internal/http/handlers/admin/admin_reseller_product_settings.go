package admin

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/dto"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type AdminResellerProductSettingRequest struct {
	SKUID             uint   `json:"sku_id"`
	IsListed          bool   `json:"is_listed"`
	PricingMode       string `json:"pricing_mode"`
	MarkupPercent     string `json:"markup_percent"`
	FixedMarkupAmount string `json:"fixed_markup_amount"`
	FixedPriceAmount  string `json:"fixed_price_amount"`
	SortOrder         int    `json:"sort_order"`
}

type AdminResellerProductSettingsUpdateRequest struct {
	Settings []AdminResellerProductSettingRequest `json:"settings"`
}

func (req AdminResellerProductSettingsUpdateRequest) toServiceInput() (service.ResellerProductSettingSaveInput, error) {
	input := service.ResellerProductSettingSaveInput{Settings: make([]service.ResellerProductSettingInput, 0, len(req.Settings))}
	for _, item := range req.Settings {
		markup, err := parseAdminResellerProductSettingDecimalField(item.MarkupPercent)
		if err != nil {
			return input, err
		}
		fixedMarkup, err := parseAdminResellerProductSettingDecimalField(item.FixedMarkupAmount)
		if err != nil {
			return input, err
		}
		fixedPrice, err := parseAdminResellerProductSettingDecimalField(item.FixedPriceAmount)
		if err != nil {
			return input, err
		}
		input.Settings = append(input.Settings, service.ResellerProductSettingInput{
			SKUID:             item.SKUID,
			IsListed:          item.IsListed,
			PricingMode:       strings.TrimSpace(item.PricingMode),
			MarkupPercent:     markup,
			FixedMarkupAmount: fixedMarkup,
			FixedPriceAmount:  fixedPrice,
			SortOrder:         item.SortOrder,
		})
	}
	return input, nil
}

func parseAdminResellerProductSettingDecimalField(raw string) (decimal.Decimal, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(value)
}

// ListResellerProductSettings 管理端分销商品配置列表。
func (h *Handler) ListResellerProductSettings(c *gin.Context) {
	if h.ResellerProductSettingService == nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	page, pageSize := shared.ParsePagination(c)
	resellerID, _ := shared.ParseQueryUint(c.Query("reseller_id"), false)
	userID, _ := shared.ParseQueryUint(c.Query("user_id"), false)
	productID, _ := shared.ParseQueryUint(c.Query("product_id"), false)
	rows, total, err := h.ResellerProductSettingService.ListAdminSettings(service.ResellerProductSettingAdminListInput{
		Page:        page,
		PageSize:    pageSize,
		ResellerID:  resellerID,
		UserID:      userID,
		ProductID:   productID,
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		PricingMode: strings.TrimSpace(c.Query("pricing_mode")),
		Configured:  strings.TrimSpace(c.Query("configured")),
		Listed:      strings.TrimSpace(c.Query("listed")),
	})
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, dto.NewAdminResellerProductSettingRespList(rows), response.BuildPagination(page, pageSize, total))
}

// GetResellerProductSetting 管理端查看某分销商的商品配置详情。
func (h *Handler) GetResellerProductSetting(c *gin.Context) {
	if h.ResellerProductSettingService == nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	resellerID, productID, ok := parseResellerProductSettingParams(c)
	if !ok {
		return
	}
	detail, err := h.ResellerProductSettingService.GetAdminProductSetting(resellerID, productID)
	if err != nil {
		respondAdminResellerProductSettingError(c, err)
		return
	}
	response.Success(c, dto.NewResellerProductSettingDetailResp(adminResellerProductSettingDTOInputFromDetail(detail)))
}

// UpdateResellerProductSettings 管理端保存某分销商的商品配置。
func (h *Handler) UpdateResellerProductSettings(c *gin.Context) {
	if h.ResellerProductSettingService == nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	resellerID, productID, ok := parseResellerProductSettingParams(c)
	if !ok {
		return
	}
	var req AdminResellerProductSettingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	input, err := req.toServiceInput()
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	detail, err := h.ResellerProductSettingService.SaveAdminProductSettings(resellerID, productID, input)
	if err != nil {
		respondAdminResellerProductSettingError(c, err)
		return
	}
	h.recordResellerAudit(c, "reseller_product_setting_save", "/admin/resellers/product-settings/:reseller_id/:product_id", gin.H{
		"reseller_id":    resellerID,
		"product_id":     productID,
		"sku_ids":        collectResellerProductSettingRequestSKUIDs(req.Settings),
		"settings_count": len(req.Settings),
	})
	response.Success(c, dto.NewResellerProductSettingDetailResp(adminResellerProductSettingDTOInputFromDetail(detail)))
}

// PreviewResellerProductSettings 管理端计算某分销商拟用定价规则的预计生效价与校验结果（不落库）。
func (h *Handler) PreviewResellerProductSettings(c *gin.Context) {
	if h.ResellerProductSettingService == nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	resellerID, productID, ok := parseResellerProductSettingParams(c)
	if !ok {
		return
	}
	var req AdminResellerProductSettingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	input, err := req.toServiceInput()
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	items, err := h.ResellerProductSettingService.PreviewAdminProductSettings(resellerID, productID, input)
	if err != nil {
		respondAdminResellerProductSettingError(c, err)
		return
	}
	previews := make([]dto.ResellerProductSettingPreviewInput, 0, len(items))
	for _, item := range items {
		previews = append(previews, dto.ResellerProductSettingPreviewInput{
			SKUID:          item.SKUID,
			IsListed:       item.IsListed,
			BasePrice:      item.BasePrice.StringFixed(2),
			EffectivePrice: item.EffectivePrice.StringFixed(2),
			Valid:          item.Valid,
			ErrorCode:      item.ErrorCode,
		})
	}
	response.Success(c, dto.NewResellerProductSettingPreviewResp(previews))
}

// ResetResellerProductSetting 管理端删除某分销商的商品级或 SKU 级配置。
func (h *Handler) ResetResellerProductSetting(c *gin.Context) {
	if h.ResellerProductSettingService == nil {
		shared.RespondError(c, response.CodeInternal, "error.user_fetch_failed", nil)
		return
	}
	resellerID, productID, ok := parseResellerProductSettingParams(c)
	if !ok {
		return
	}
	skuID, err := shared.ParseQueryUint(c.Query("sku_id"), false)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	if err := h.ResellerProductSettingService.ResetAdminProductSetting(resellerID, productID, skuID); err != nil {
		respondAdminResellerProductSettingError(c, err)
		return
	}
	h.recordResellerAudit(c, "reseller_product_setting_reset", "/admin/resellers/product-settings/:reseller_id/:product_id", gin.H{
		"reseller_id": resellerID,
		"product_id":  productID,
		"sku_ids":     []uint{skuID},
	})
	response.Success(c, gin.H{"deleted": true})
}

func parseResellerProductSettingParams(c *gin.Context) (uint, uint, bool) {
	resellerID, err := shared.ParseParamUint(c, "reseller_id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return 0, 0, false
	}
	productID, err := shared.ParseParamUint(c, "product_id")
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return 0, 0, false
	}
	return resellerID, productID, true
}

func collectResellerProductSettingRequestSKUIDs(settings []AdminResellerProductSettingRequest) []uint {
	out := make([]uint, 0, len(settings))
	for _, setting := range settings {
		out = append(out, setting.SKUID)
	}
	return out
}

func adminResellerProductSettingDTOInputFromDetail(detail *service.ResellerProductSettingDetail) dto.ResellerProductSettingDTOInput {
	if detail == nil {
		return dto.ResellerProductSettingDTOInput{}
	}
	return dto.ResellerProductSettingDTOInput{
		Product:          detail.Product,
		Settings:         detail.Settings,
		EffectiveBySKUID: adminResellerDecimalMapToStringMap(detail.EffectiveBySKUID),
		RuleBySKUID:      detail.RuleBySKUID,
	}
}

func adminResellerDecimalMapToStringMap(input map[uint]decimal.Decimal) map[uint]string {
	out := make(map[uint]string, len(input))
	for key, value := range input {
		out[key] = value.StringFixed(2)
	}
	return out
}

func respondAdminResellerProductSettingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		shared.RespondError(c, response.CodeNotFound, "error.not_found", nil)
	case errors.Is(err, service.ErrResellerProfileInactive):
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
	case errors.Is(err, service.ErrProductSKUInvalid):
		shared.RespondError(c, response.CodeBadRequest, "error.order_item_invalid", nil)
	case errors.Is(err, service.ErrResellerPriceBelowBase):
		shared.RespondError(c, response.CodeBadRequest, "error.reseller_price_invalid", nil)
	case errors.Is(err, service.ErrResellerMarkupExceeded):
		shared.RespondError(c, response.CodeBadRequest, "error.reseller_markup_exceeded", nil)
	case errors.Is(err, service.ErrResellerPricingModeInvalid):
		shared.RespondError(c, response.CodeBadRequest, "error.reseller_price_invalid", nil)
	default:
		shared.RespondError(c, response.CodeInternal, "error.save_failed", err)
	}
}
