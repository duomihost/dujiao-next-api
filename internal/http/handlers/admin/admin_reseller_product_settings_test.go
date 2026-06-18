package admin

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func setupAdminResellerProductSettingHandlerTest(t *testing.T) (*Handler, *gorm.DB) {
	h, db := setupAdminResellerManagementHandlerTest(t)
	if err := db.AutoMigrate(&models.Category{}, &models.Product{}, &models.ProductSKU{}, &models.ResellerProductSetting{}); err != nil {
		t.Fatalf("migrate product setting tables failed: %v", err)
	}
	settingRepo := repository.NewResellerProductSettingRepository(db)
	resellerRepo := repository.NewResellerRepository(db)
	productRepo := repository.NewProductRepository(db)
	h.Container.ResellerProductSettingRepo = settingRepo
	h.Container.ResellerProductSettingService = service.NewResellerProductSettingService(settingRepo, resellerRepo, productRepo)
	return h, db
}

func TestAdminResellerProductSettingsSaveCreatesAudit(t *testing.T) {
	h, db := setupAdminResellerProductSettingHandlerTest(t)
	profile := seedAdminResellerManagementProfile(t, db, models.ResellerProfileStatusActive)
	product, skus := seedResellerProductSettingProductForAdminHandler(t, db)
	body := fmt.Sprintf(`{"settings":[{"sku_id":%d,"is_listed":true,"pricing_mode":"fixed_price","fixed_price_amount":"130.00"}]}`, skus[0].ID)
	c, recorder := newAdminResellerManagementContext(http.MethodPut, fmt.Sprintf("/admin/resellers/product-settings/%d/%d", profile.ID, product.ID), strings.NewReader(body))
	c.Params = gin.Params{{Key: "reseller_id", Value: fmt.Sprintf("%d", profile.ID)}, {Key: "product_id", Value: fmt.Sprintf("%d", product.ID)}}

	h.UpdateResellerProductSettings(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var auditCount int64
	if err := db.Model(&models.AuthzAuditLog{}).Where("action = ?", "reseller_product_setting_save").Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit failed: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one audit, got %d", auditCount)
	}
}

func TestAdminResellerProductSettingsListFiltersByReseller(t *testing.T) {
	h, db := setupAdminResellerProductSettingHandlerTest(t)
	profile := seedAdminResellerManagementProfile(t, db, models.ResellerProfileStatusActive)
	other := seedAdminResellerManagementProfile(t, db, models.ResellerProfileStatusActive)
	product, skus := seedResellerProductSettingProductForAdminHandler(t, db)
	repo := repository.NewResellerProductSettingRepository(db)
	if _, err := repo.UpsertSetting(models.ResellerProductSetting{ResellerID: profile.ID, ProductID: product.ID, SKUID: skus[0].ID, IsListed: true, PricingMode: models.ResellerPricingModeFixedPrice, FixedPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("130.00"))}); err != nil {
		t.Fatalf("upsert setting failed: %v", err)
	}
	if _, err := repo.UpsertSetting(models.ResellerProductSetting{ResellerID: other.ID, ProductID: product.ID, SKUID: skus[0].ID, IsListed: true, PricingMode: models.ResellerPricingModeFixedPrice, FixedPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("140.00"))}); err != nil {
		t.Fatalf("upsert other setting failed: %v", err)
	}
	c, recorder := newAdminResellerManagementContext(http.MethodGet, fmt.Sprintf("/admin/resellers/product-settings?reseller_id=%d", profile.ID), nil)

	h.ListResellerProductSettings(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), fmt.Sprintf(`"reseller_id":%d`, profile.ID)) || strings.Contains(recorder.Body.String(), fmt.Sprintf(`"reseller_id":%d`, other.ID)) {
		t.Fatalf("unexpected list body: %s", recorder.Body.String())
	}
}

func TestAdminResellerProductSettingsSaveReturnsPrecisePricingError(t *testing.T) {
	h, db := setupAdminResellerProductSettingHandlerTest(t)
	profile := seedAdminResellerManagementProfile(t, db, models.ResellerProfileStatusActive)
	product, skus := seedResellerProductSettingProductForAdminHandler(t, db)
	body := fmt.Sprintf(`{"settings":[{"sku_id":%d,"is_listed":true,"pricing_mode":"fixed_price","fixed_price_amount":"99.99"}]}`, skus[0].ID)
	c, recorder := newAdminResellerManagementContext(http.MethodPut, fmt.Sprintf("/admin/resellers/product-settings/%d/%d", profile.ID, product.ID), strings.NewReader(body))
	c.Params = gin.Params{{Key: "reseller_id", Value: fmt.Sprintf("%d", profile.ID)}, {Key: "product_id", Value: fmt.Sprintf("%d", product.ID)}}

	h.UpdateResellerProductSettings(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected wrapped error response 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status_code":400`) ||
		!strings.Contains(recorder.Body.String(), "分销商品价格配置不合法") {
		t.Fatalf("expected precise pricing error, body=%s", recorder.Body.String())
	}
}

func seedResellerProductSettingProductForAdminHandler(t *testing.T, db *gorm.DB) (models.Product, []models.ProductSKU) {
	t.Helper()
	category := models.Category{Slug: fmt.Sprintf("admin-setting-cat-%d", time.Now().UnixNano()), NameJSON: models.JSON{"zh-CN": "分类"}, IsActive: true}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}
	product := models.Product{
		CategoryID:      category.ID,
		Slug:            fmt.Sprintf("admin-setting-product-%d", time.Now().UnixNano()),
		TitleJSON:       models.JSON{"zh-CN": "后台商品", "zh-TW": "後台商品", "en-US": "Admin Product"},
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
