package admin

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

func TestAdminResellerSiteConfigUpdateAndAudit(t *testing.T) {
	h, db := setupAdminResellerManagementHandlerTest(t)
	resellerRepo := repository.NewResellerRepository(db)
	h.ResellerSiteConfigService = service.NewResellerSiteConfigService(resellerRepo)
	profile := seedAdminResellerManagementProfile(t, db, models.ResellerProfileStatusActive)
	body := strings.NewReader(`{"site_name":"Admin Edited","support":{"telegram":"https://t.me/admin"}}`)
	c, recorder := newAdminResellerManagementContext(http.MethodPut, fmt.Sprintf("/admin/resellers/site-configs/%d", profile.ID), body)
	c.Params = gin.Params{{Key: "reseller_id", Value: fmt.Sprintf("%d", profile.ID)}}

	h.UpdateResellerSiteConfig(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var row models.ResellerSiteConfig
	if err := db.Where("reseller_id = ?", profile.ID).First(&row).Error; err != nil {
		t.Fatalf("expected saved config: %v", err)
	}
	if row.SiteName != "Admin Edited" {
		t.Fatalf("unexpected site name: %s", row.SiteName)
	}
	var auditCount int64
	if err := db.Model(&models.AuthzAuditLog{}).Where("action = ?", "reseller_site_config_update").Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit failed: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one audit log, got %d", auditCount)
	}
}

func TestAdminResellerSiteConfigResetAndList(t *testing.T) {
	h, db := setupAdminResellerManagementHandlerTest(t)
	resellerRepo := repository.NewResellerRepository(db)
	h.ResellerSiteConfigService = service.NewResellerSiteConfigService(resellerRepo)
	profile := seedAdminResellerManagementProfile(t, db, models.ResellerProfileStatusActive)
	if _, err := resellerRepo.UpsertSiteConfig(models.ResellerSiteConfig{ResellerID: profile.ID, SiteName: "Reset Me"}); err != nil {
		t.Fatalf("create site config failed: %v", err)
	}

	c, recorder := newAdminResellerManagementContext(http.MethodPost, fmt.Sprintf("/admin/resellers/site-configs/%d/reset", profile.ID), strings.NewReader(`{}`))
	c.Params = gin.Params{{Key: "reseller_id", Value: fmt.Sprintf("%d", profile.ID)}}
	h.ResetResellerSiteConfig(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	loaded, err := resellerRepo.GetSiteConfigByResellerID(profile.ID)
	if err != nil {
		t.Fatalf("reload config failed: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected config reset to delete row, got %+v", loaded)
	}

	c, recorder = newAdminResellerManagementContext(http.MethodGet, "/admin/resellers/site-configs?page=1&page_size=20", nil)
	h.ListResellerSiteConfigs(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
