package public

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/provider"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func newPublicResellerProductSettingHandlerForTest(db *gorm.DB) *Handler {
	resellerRepo := repository.NewResellerRepository(db)
	settingRepo := repository.NewResellerProductSettingRepository(db)
	productRepo := repository.NewProductRepository(db)
	return &Handler{Container: &provider.Container{
		ResellerProductSettingService: service.NewResellerProductSettingService(settingRepo, resellerRepo, productRepo),
	}}
}

func TestPublicResellerProductSettingsSaveUsesCurrentUserProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openPublicResellerHandlerTestDB(t)
	if err := db.AutoMigrate(&models.Category{}, &models.Product{}, &models.ProductSKU{}, &models.ResellerProductSetting{}); err != nil {
		t.Fatalf("migrate product setting tables failed: %v", err)
	}
	profile := seedPublicResellerHandlerProfile(t, db)
	product, skus := seedResellerProductSettingProductForPublicHandler(t, db)
	h := newPublicResellerProductSettingHandlerForTest(db)

	body := fmt.Sprintf(`{"settings":[{"sku_id":%d,"is_listed":true,"pricing_mode":"fixed_price","fixed_price_amount":"130.00"}]}`, skus[0].ID)
	c, recorder := newPublicResellerHandlerTestContext(http.MethodPut, fmt.Sprintf("/api/v1/reseller/product-settings/%d", product.ID), []byte(body), profile.UserID)
	c.Params = gin.Params{{Key: "product_id", Value: fmt.Sprintf("%d", product.ID)}}

	h.UpdateResellerProductSettings(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"fixed_price_amount":"130"`) && !strings.Contains(recorder.Body.String(), `"fixed_price_amount":"130.00"`) {
		t.Fatalf("expected fixed price in body: %s", recorder.Body.String())
	}
	var count int64
	if err := db.Model(&models.ResellerProductSetting{}).Where("reseller_id = ?", profile.ID).Count(&count).Error; err != nil {
		t.Fatalf("count setting failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one setting for current reseller, got %d", count)
	}
}

func TestPublicResellerProductSettingsRejectInactiveProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openPublicResellerHandlerTestDB(t)
	if err := db.AutoMigrate(&models.Category{}, &models.Product{}, &models.ProductSKU{}, &models.ResellerProductSetting{}); err != nil {
		t.Fatalf("migrate product setting tables failed: %v", err)
	}
	user := seedPublicResellerHandlerUser(t, db)
	profile := models.ResellerProfile{UserID: user.ID, Status: models.ResellerProfileStatusPendingReview, SettlementStatus: models.ResellerSettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	product, _ := seedResellerProductSettingProductForPublicHandler(t, db)
	h := newPublicResellerProductSettingHandlerForTest(db)
	c, recorder := newPublicResellerHandlerTestContext(http.MethodGet, fmt.Sprintf("/api/v1/reseller/product-settings/%d", product.ID), nil, user.ID)
	c.Params = gin.Params{{Key: "product_id", Value: fmt.Sprintf("%d", product.ID)}}

	h.GetResellerProductSetting(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected http 200 envelope, got %d", recorder.Code)
	}
	var resp struct {
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected status_code 400, got body=%s", recorder.Body.String())
	}
}

func seedResellerProductSettingProductForPublicHandler(t *testing.T, db *gorm.DB) (models.Product, []models.ProductSKU) {
	t.Helper()
	category := models.Category{Slug: fmt.Sprintf("public-setting-cat-%d", time.Now().UnixNano()), NameJSON: models.JSON{"zh-CN": "分类"}, IsActive: true}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}
	product := models.Product{
		CategoryID:      category.ID,
		Slug:            fmt.Sprintf("public-setting-product-%d", time.Now().UnixNano()),
		TitleJSON:       models.JSON{"zh-CN": "公开商品", "zh-TW": "公開商品", "en-US": "Public Product"},
		PriceAmount:     models.NewMoneyFromDecimal(decimal.RequireFromString("100.00")),
		CostPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("80.00")),
		IsActive:        true,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	skus := []models.ProductSKU{
		{ProductID: product.ID, SKUCode: "A", PriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("100.00")), CostPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("80.00")), IsActive: true},
	}
	if err := db.Create(&skus).Error; err != nil {
		t.Fatalf("create skus failed: %v", err)
	}
	return product, skus
}
