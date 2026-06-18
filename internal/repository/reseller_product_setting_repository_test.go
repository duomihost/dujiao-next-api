package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func openResellerProductSettingRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:reseller_product_setting_repo_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Category{},
		&models.Product{},
		&models.ProductSKU{},
		&models.ResellerProfile{},
		&models.ResellerProductSetting{},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}

func seedResellerProductSettingProfile(t *testing.T, db *gorm.DB, email string) models.ResellerProfile {
	t.Helper()
	user := models.User{Email: email, PasswordHash: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	profile := models.ResellerProfile{
		UserID:               user.ID,
		Status:               models.ResellerProfileStatusActive,
		DefaultMarkupPercent: models.NewMoneyFromDecimal(decimal.RequireFromString("10.00")),
		MaxMarkupPercent:     models.NewMoneyFromDecimal(decimal.RequireFromString("50.00")),
		SettlementStatus:     models.ResellerSettlementStatusNormal,
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	return profile
}

func seedResellerProductSettingProduct(t *testing.T, db *gorm.DB, slug string) (models.Product, []models.ProductSKU) {
	t.Helper()
	category := models.Category{Slug: "cat-" + slug, NameJSON: models.JSON{"zh-CN": "分类"}, IsActive: true}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}
	product := models.Product{
		CategoryID:      category.ID,
		Slug:            slug,
		TitleJSON:       models.JSON{"zh-CN": "商品 " + slug, "zh-TW": "商品 " + slug, "en-US": "Product " + slug},
		PriceAmount:     models.NewMoneyFromDecimal(decimal.RequireFromString("100.00")),
		CostPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("80.00")),
		IsActive:        true,
		SortOrder:       10,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	skus := []models.ProductSKU{
		{
			ProductID:       product.ID,
			SKUCode:         "MONTH-1",
			SpecValuesJSON:  models.JSON{"zh-CN": "1个月", "zh-TW": "1個月", "en-US": "1 month"},
			PriceAmount:     models.NewMoneyFromDecimal(decimal.RequireFromString("100.00")),
			CostPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("80.00")),
			IsActive:        true,
			SortOrder:       20,
		},
		{
			ProductID:       product.ID,
			SKUCode:         "MONTH-3",
			SpecValuesJSON:  models.JSON{"zh-CN": "3个月", "zh-TW": "3個月", "en-US": "3 months"},
			PriceAmount:     models.NewMoneyFromDecimal(decimal.RequireFromString("250.00")),
			CostPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("180.00")),
			IsActive:        true,
			SortOrder:       10,
		},
	}
	if err := db.Create(&skus).Error; err != nil {
		t.Fatalf("create skus failed: %v", err)
	}
	return product, skus
}

func TestResellerProductSettingRepositoryUpsertRestoresSoftDeleted(t *testing.T) {
	db := openResellerProductSettingRepoTestDB(t)
	profile := seedResellerProductSettingProfile(t, db, "restore@example.test")
	product, skus := seedResellerProductSettingProduct(t, db, "restore-product")
	repo := NewResellerProductSettingRepository(db)

	created, err := repo.UpsertSetting(models.ResellerProductSetting{
		ResellerID:        profile.ID,
		ProductID:         product.ID,
		SKUID:             skus[0].ID,
		IsListed:          true,
		PricingMode:       models.ResellerPricingModeFixedPrice,
		FixedPriceAmount:  models.NewMoneyFromDecimal(decimal.RequireFromString("128.00")),
		MarkupPercent:     models.NewMoneyFromDecimal(decimal.Zero),
		FixedMarkupAmount: models.NewMoneyFromDecimal(decimal.Zero),
	})
	if err != nil {
		t.Fatalf("upsert create failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected created id")
	}
	if err := repo.DeleteSetting(profile.ID, product.ID, skus[0].ID); err != nil {
		t.Fatalf("delete setting failed: %v", err)
	}
	restored, err := repo.UpsertSetting(models.ResellerProductSetting{
		ResellerID:        profile.ID,
		ProductID:         product.ID,
		SKUID:             skus[0].ID,
		IsListed:          true,
		PricingMode:       models.ResellerPricingModeFixedMarkup,
		FixedMarkupAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("15.00")),
		MarkupPercent:     models.NewMoneyFromDecimal(decimal.Zero),
		FixedPriceAmount:  models.NewMoneyFromDecimal(decimal.Zero),
	})
	if err != nil {
		t.Fatalf("upsert restore failed: %v", err)
	}
	if restored.ID != created.ID || restored.DeletedAt.Valid || !restored.DeletedAt.Time.IsZero() {
		t.Fatalf("expected restored same row with deleted_at reset, got created=%d restored=%d deleted_at=%v", created.ID, restored.ID, restored.DeletedAt)
	}
	if restored.PricingMode != models.ResellerPricingModeFixedMarkup || restored.FixedMarkupAmount.String() != "15.00" {
		t.Fatalf("unexpected restored row: %+v", restored)
	}
}

func TestResellerProductSettingRepositoryEnforcesScopeUniqueness(t *testing.T) {
	db := openResellerProductSettingRepoTestDB(t)
	profile := seedResellerProductSettingProfile(t, db, "unique@example.test")
	product, skus := seedResellerProductSettingProduct(t, db, "unique-product")
	repo := NewResellerProductSettingRepository(db)

	if _, err := repo.UpsertSetting(models.ResellerProductSetting{
		ResellerID:        profile.ID,
		ProductID:         product.ID,
		SKUID:             skus[0].ID,
		IsListed:          true,
		PricingMode:       models.ResellerPricingModeInherit,
		MarkupPercent:     models.NewMoneyFromDecimal(decimal.RequireFromString("0.00")),
		FixedMarkupAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("0.00")),
		FixedPriceAmount:  models.NewMoneyFromDecimal(decimal.RequireFromString("0.00")),
	}); err != nil {
		t.Fatalf("create initial setting failed: %v", err)
	}

	err := db.Create(&models.ResellerProductSetting{
		ResellerID:        profile.ID,
		ProductID:         product.ID,
		SKUID:             skus[0].ID,
		IsListed:          true,
		PricingMode:       models.ResellerPricingModeFixedPrice,
		MarkupPercent:     models.NewMoneyFromDecimal(decimal.RequireFromString("0.00")),
		FixedMarkupAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("0.00")),
		FixedPriceAmount:  models.NewMoneyFromDecimal(decimal.RequireFromString("150.00")),
	}).Error
	if err == nil {
		t.Fatal("expected duplicate active setting insert to fail")
	}
}

func TestResellerProductSettingRepositoryListProductsWithSettings(t *testing.T) {
	db := openResellerProductSettingRepoTestDB(t)
	profile := seedResellerProductSettingProfile(t, db, "list@example.test")
	product, skus := seedResellerProductSettingProduct(t, db, "searchable-product")
	otherProfile := seedResellerProductSettingProfile(t, db, "other@example.test")
	repo := NewResellerProductSettingRepository(db)
	if _, err := repo.UpsertSetting(models.ResellerProductSetting{
		ResellerID:       profile.ID,
		ProductID:        product.ID,
		SKUID:            0,
		IsListed:         true,
		PricingMode:      models.ResellerPricingModeMarkupPercent,
		MarkupPercent:    models.NewMoneyFromDecimal(decimal.RequireFromString("20.00")),
		FixedPriceAmount: models.NewMoneyFromDecimal(decimal.Zero),
	}); err != nil {
		t.Fatalf("upsert product setting failed: %v", err)
	}
	if _, err := repo.UpsertSetting(models.ResellerProductSetting{
		ResellerID:       otherProfile.ID,
		ProductID:        product.ID,
		SKUID:            skus[0].ID,
		IsListed:         true,
		PricingMode:      models.ResellerPricingModeFixedPrice,
		FixedPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("180.00")),
	}); err != nil {
		t.Fatalf("upsert other setting failed: %v", err)
	}

	rows, total, err := repo.ListProductsWithSettings(ResellerProductSettingListFilter{
		Page:       1,
		PageSize:   20,
		ResellerID: profile.ID,
		Keyword:    "searchable",
		Configured: "configured",
	})
	if err != nil {
		t.Fatalf("list products failed: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected one product row, total=%d len=%d", total, len(rows))
	}
	if rows[0].Product.ID != product.ID || len(rows[0].Product.SKUs) != 2 {
		t.Fatalf("unexpected product row: %+v", rows[0])
	}
	if len(rows[0].Settings) != 1 || rows[0].Settings[0].ResellerID != profile.ID || rows[0].Settings[0].SKUID != 0 {
		t.Fatalf("expected only current reseller settings, got %+v", rows[0].Settings)
	}
}

func TestResellerProductSettingRepositoryGetProductSettings(t *testing.T) {
	db := openResellerProductSettingRepoTestDB(t)
	profile := seedResellerProductSettingProfile(t, db, "detail@example.test")
	product, skus := seedResellerProductSettingProduct(t, db, "detail-product")
	repo := NewResellerProductSettingRepository(db)
	if _, err := repo.UpsertSetting(models.ResellerProductSetting{
		ResellerID:       profile.ID,
		ProductID:        product.ID,
		SKUID:            skus[1].ID,
		IsListed:         true,
		PricingMode:      models.ResellerPricingModeFixedPrice,
		FixedPriceAmount: models.NewMoneyFromDecimal(decimal.RequireFromString("300.00")),
	}); err != nil {
		t.Fatalf("upsert setting failed: %v", err)
	}
	row, err := repo.GetProductWithSettings(profile.ID, product.ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	if row == nil || row.Product.ID != product.ID || len(row.Settings) != 1 || row.Settings[0].SKUID != skus[1].ID {
		t.Fatalf("unexpected detail row: %+v", row)
	}
}
