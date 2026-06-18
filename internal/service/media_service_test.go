package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupMediaServiceTest(t *testing.T) (*MediaService, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:media_service_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.Media{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	return NewMediaService(repository.NewMediaRepository(db)), db
}

func createMediaFile(t *testing.T, path string) {
	t.Helper()
	diskPath := filepath.FromSlash(path)
	if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
		t.Fatalf("create media dir failed: %v", err)
	}
	if err := os.WriteFile(diskPath, []byte("image"), 0644); err != nil {
		t.Fatalf("write media file failed: %v", err)
	}
}

func TestMediaServiceBatchDeleteDeletesFilesAndReportsFailures(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)

	svc, db := setupMediaServiceTest(t)
	createMediaFile(t, "uploads/common/first.png")
	createMediaFile(t, "uploads/common/second.png")

	first := models.Media{
		Name:     "first",
		Filename: "first.png",
		Path:     "/uploads/common/first.png",
		MimeType: "image/png",
		Size:     5,
		Scene:    "common",
	}
	second := models.Media{
		Name:     "second",
		Filename: "second.png",
		Path:     "/uploads/common/second.png",
		MimeType: "image/png",
		Size:     5,
		Scene:    "common",
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first media failed: %v", err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("create second media failed: %v", err)
	}

	successCount, failedIDs := svc.BatchDelete([]uint{first.ID, 9999, second.ID})

	if successCount != 2 {
		t.Fatalf("success count want 2 got %d", successCount)
	}
	if len(failedIDs) != 1 || failedIDs[0] != 9999 {
		t.Fatalf("failed IDs want [9999] got %#v", failedIDs)
	}
	if _, err := os.Stat(filepath.FromSlash("uploads/common/first.png")); !os.IsNotExist(err) {
		t.Fatalf("expected first file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.FromSlash("uploads/common/second.png")); !os.IsNotExist(err) {
		t.Fatalf("expected second file to be removed, stat err=%v", err)
	}

	var remaining int64
	if err := db.Model(&models.Media{}).Count(&remaining).Error; err != nil {
		t.Fatalf("count media failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected selected media to be soft-deleted, remaining=%d", remaining)
	}
}
