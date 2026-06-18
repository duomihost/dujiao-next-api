package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/provider"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAdminMediaHandlerTest(t *testing.T) (*Handler, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:admin_media_handler_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.Media{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	mediaService := service.NewMediaService(repository.NewMediaRepository(db))
	return &Handler{Container: &provider.Container{MediaService: mediaService}}, db
}

func TestBatchDeleteMediaReturnsCounts(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)

	h, db := setupAdminMediaHandlerTest(t)
	if err := os.MkdirAll(filepath.FromSlash("uploads/common"), 0755); err != nil {
		t.Fatalf("create uploads dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.FromSlash("uploads/common/asset.png"), []byte("image"), 0644); err != nil {
		t.Fatalf("write media file failed: %v", err)
	}

	media := models.Media{
		Name:     "asset",
		Filename: "asset.png",
		Path:     "/uploads/common/asset.png",
		MimeType: "image/png",
		Size:     5,
		Scene:    "common",
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatalf("create media failed: %v", err)
	}

	body := fmt.Sprintf(`{"ids":[%d,9999]}`, media.ID)
	req := httptest.NewRequest(http.MethodPost, "/admin/media/batch-delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.BatchDeleteMedia(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d body=%s", w.Code, w.Body.String())
	}
	var got response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, w.Body.String())
	}
	if got.StatusCode != response.CodeOK {
		t.Fatalf("business status want success got %d body=%s", got.StatusCode, w.Body.String())
	}

	data, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("response data type mismatch: %#v", got.Data)
	}
	if total := int(data["total"].(float64)); total != 2 {
		t.Fatalf("total want 2 got %d", total)
	}
	if success := int(data["success_count"].(float64)); success != 1 {
		t.Fatalf("success count want 1 got %d", success)
	}
	failedIDs := data["failed_ids"].([]any)
	if len(failedIDs) != 1 || int(failedIDs[0].(float64)) != 9999 {
		t.Fatalf("failed IDs want [9999] got %#v", failedIDs)
	}
}
