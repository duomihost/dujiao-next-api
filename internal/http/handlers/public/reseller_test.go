package public

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/provider"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func openPublicResellerHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:public_reseller_handler_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.ResellerProfile{},
		&models.ResellerDomain{},
		&models.ResellerLedgerEntry{},
		&models.ResellerWithdrawRequest{},
		&models.ResellerBalanceAccount{},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}

func newPublicResellerHandlerTestContext(method, path string, body []byte, userID uint) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set("user_id", userID)
	return c, recorder
}

func seedPublicResellerHandlerProfile(t *testing.T, db *gorm.DB) models.ResellerProfile {
	t.Helper()
	user := models.User{
		Email:        fmt.Sprintf("public-reseller-%d@example.test", time.Now().UnixNano()),
		PasswordHash: "hash",
		Status:       constants.UserStatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	profile := models.ResellerProfile{
		UserID:           user.ID,
		Status:           models.ResellerProfileStatusActive,
		SettlementStatus: models.ResellerSettlementStatusNormal,
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create reseller profile failed: %v", err)
	}
	return profile
}

func seedPublicResellerHandlerUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	user := models.User{
		Email:        fmt.Sprintf("public-reseller-user-%d@example.test", time.Now().UnixNano()),
		PasswordHash: "hash",
		Status:       constants.UserStatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	return user
}

func newPublicResellerHandlerForTest(db *gorm.DB) *Handler {
	repo := repository.NewResellerRepository(db)
	return &Handler{
		Container: &provider.Container{
			ResellerAccountingService: service.NewResellerAccountingService(repo, service.ResellerAccountingOptions{}),
		},
	}
}

func TestPublicResellerApplyAndSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openPublicResellerHandlerTestDB(t)
	user := seedPublicResellerHandlerUser(t, db)
	h := newPublicResellerHandlerForTest(db)
	h.ResellerManagementService = service.NewResellerManagementService(repository.NewResellerRepository(db), config.ResellerConfig{
		Enabled:          true,
		SelfApplyEnabled: true,
		SubdomainBase:    "shop.example.test",
		MainHosts:        []string{"main.example.test"},
	})

	payload := []byte(`{"reason":"operate a storefront"}`)
	c, recorder := newPublicResellerHandlerTestContext(http.MethodPost, "/api/v1/reseller/apply", payload, user.ID)
	h.ApplyResellerProfile(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	c, recorder = newPublicResellerHandlerTestContext(http.MethodGet, "/api/v1/reseller/profile", nil, user.ID)
	h.GetResellerManagementSnapshot(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"opened":true`) || !strings.Contains(recorder.Body.String(), `"pending_review"`) {
		t.Fatalf("unexpected snapshot body: %s", recorder.Body.String())
	}
}

func TestPublicResellerSubmitCustomDomainRequiresActiveProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openPublicResellerHandlerTestDB(t)
	user := seedPublicResellerHandlerUser(t, db)
	h := newPublicResellerHandlerForTest(db)
	h.ResellerManagementService = service.NewResellerManagementService(repository.NewResellerRepository(db), config.ResellerConfig{
		Enabled:          true,
		SelfApplyEnabled: true,
		SubdomainBase:    "shop.example.test",
		MainHosts:        []string{"main.example.test"},
	})
	if _, err := h.ResellerManagementService.ApplyUserReseller(user.ID, service.ResellerApplyInput{Reason: "pending"}); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	c, recorder := newPublicResellerHandlerTestContext(http.MethodPost, "/api/v1/reseller/domains", []byte(`{"domain":"shop.customer.example"}`), user.ID)
	h.SubmitResellerCustomDomain(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected http 200 envelope, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.StatusCode != response.CodeBadRequest {
		t.Fatalf("expected status_code=400 for inactive profile, got %+v body=%s", resp, recorder.Body.String())
	}
}

func TestPublicResellerFinanceDashboard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openPublicResellerHandlerTestDB(t)
	profile := seedPublicResellerHandlerProfile(t, db)
	if err := db.Create(&models.ResellerBalanceAccount{
		ResellerID:           profile.ID,
		Currency:             "USD",
		Status:               models.ResellerBalanceStatusNormal,
		AvailableAmountCache: models.NewMoneyFromDecimal(decimal.RequireFromString("120.50")),
		LockedAmountCache:    models.NewMoneyFromDecimal(decimal.RequireFromString("10.00")),
		NegativeAmountCache:  models.NewMoneyFromDecimal(decimal.Zero),
	}).Error; err != nil {
		t.Fatalf("create balance account failed: %v", err)
	}

	h := newPublicResellerHandlerForTest(db)
	c, recorder := newPublicResellerHandlerTestContext(http.MethodGet, "/api/v1/reseller/dashboard", nil, profile.UserID)

	h.GetResellerDashboard(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected http 200, got %d", recorder.Code)
	}
	var resp struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			Opened   bool `json:"opened"`
			Profile  any  `json:"profile"`
			Balances []struct {
				Currency        string `json:"currency"`
				AvailableAmount string `json:"available_amount"`
			} `json:"balances"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.StatusCode != response.CodeOK {
		t.Fatalf("expected status_code=0, got %d body=%s", resp.StatusCode, recorder.Body.String())
	}
	if !resp.Data.Opened || resp.Data.Profile == nil {
		t.Fatalf("expected opened dashboard with profile, got %+v", resp.Data)
	}
	if len(resp.Data.Balances) != 1 || resp.Data.Balances[0].Currency != "USD" || resp.Data.Balances[0].AvailableAmount != "120.50" {
		t.Fatalf("unexpected balances: %+v", resp.Data.Balances)
	}
}

func TestPublicResellerFinanceApplyWithdraw(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openPublicResellerHandlerTestDB(t)
	profile := seedPublicResellerHandlerProfile(t, db)
	now := time.Now()
	if err := db.Create(&models.ResellerLedgerEntry{
		ResellerID:     profile.ID,
		Type:           models.ResellerLedgerTypeOrderProfit,
		Amount:         models.NewMoneyFromDecimal(decimal.RequireFromString("60.00")),
		Currency:       "USD",
		IdempotencyKey: "public-withdraw-ledger-1",
		Status:         models.ResellerLedgerStatusAvailable,
		AvailableAt:    &now,
	}).Error; err != nil {
		t.Fatalf("create ledger entry failed: %v", err)
	}
	if err := db.Create(&models.ResellerBalanceAccount{
		ResellerID:           profile.ID,
		Currency:             "USD",
		Status:               models.ResellerBalanceStatusNormal,
		AvailableAmountCache: models.NewMoneyFromDecimal(decimal.RequireFromString("60.00")),
		LockedAmountCache:    models.NewMoneyFromDecimal(decimal.Zero),
		NegativeAmountCache:  models.NewMoneyFromDecimal(decimal.Zero),
	}).Error; err != nil {
		t.Fatalf("create balance account failed: %v", err)
	}

	payload := []byte(`{"amount":"25.00","currency":"USD","channel":"USDT","account":"T-address"}`)
	h := newPublicResellerHandlerForTest(db)
	c, recorder := newPublicResellerHandlerTestContext(http.MethodPost, "/api/v1/reseller/withdraws", payload, profile.UserID)

	h.ApplyResellerWithdraw(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected http 200, got %d", recorder.Code)
	}
	var resp struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.StatusCode != response.CodeOK {
		t.Fatalf("expected status_code=0, got %d body=%s", resp.StatusCode, recorder.Body.String())
	}
	if resp.Data.Amount != "25.00" || resp.Data.Currency != "USD" || resp.Data.Status != models.ResellerWithdrawStatusPending {
		t.Fatalf("unexpected withdraw response: %+v", resp.Data)
	}
	var count int64
	if err := db.Model(&models.ResellerWithdrawRequest{}).Where("reseller_id = ?", profile.ID).Count(&count).Error; err != nil {
		t.Fatalf("count withdraw requests failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one withdraw request, got %d", count)
	}
}
