package repository

import (
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAdminRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Admin{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestAdminRepository_UpdatePassword_RewritesHashAndForcesLogout(t *testing.T) {
	db := setupAdminRepositoryTestDB(t)
	repo := NewAdminRepository(db)

	admin := &models.Admin{
		Username:     "super",
		PasswordHash: "old-hash",
		IsSuper:      true,
		TokenVersion: 3,
	}
	if err := repo.Create(admin); err != nil {
		t.Fatalf("create: %v", err)
	}

	before := time.Now().Add(-time.Second)
	if err := repo.UpdatePassword(admin.ID, "new-hash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	after := time.Now().Add(time.Second)

	got, err := repo.GetByID(admin.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PasswordHash != "new-hash" {
		t.Errorf("PasswordHash = %q, want new-hash", got.PasswordHash)
	}
	if got.TokenVersion != 4 {
		t.Errorf("TokenVersion = %d, want 4 (bumped from 3)", got.TokenVersion)
	}
	if got.TokenInvalidBefore == nil {
		t.Fatal("TokenInvalidBefore should be set to force logout")
	}
	if got.TokenInvalidBefore.Before(before) || got.TokenInvalidBefore.After(after) {
		t.Errorf("TokenInvalidBefore = %v, want between %v and %v", got.TokenInvalidBefore, before, after)
	}
}

func TestAdminRepository_UpdatePassword_RejectsZeroID(t *testing.T) {
	db := setupAdminRepositoryTestDB(t)
	repo := NewAdminRepository(db)

	if err := repo.UpdatePassword(0, "hash"); err == nil {
		t.Fatal("expected error for zero admin id")
	}
}

func TestAdminRepository_UpdatePassword_RejectsEmptyHash(t *testing.T) {
	db := setupAdminRepositoryTestDB(t)
	repo := NewAdminRepository(db)

	admin := &models.Admin{Username: "u", PasswordHash: "h"}
	if err := repo.Create(admin); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.UpdatePassword(admin.ID, ""); err == nil {
		t.Fatal("expected error for empty hash")
	}

	got, err := repo.GetByID(admin.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PasswordHash != "h" {
		t.Errorf("PasswordHash should be unchanged, got %q", got.PasswordHash)
	}
}
