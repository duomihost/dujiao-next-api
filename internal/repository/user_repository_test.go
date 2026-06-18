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

func setupUserRepositoryTest(t *testing.T) (*GormUserRepository, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:user_repository_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.WalletAccount{}); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return NewUserRepository(db), db
}

func createTestUser(t *testing.T, db *gorm.DB, email string, createdAt time.Time, lastLogin *time.Time) *models.User {
	t.Helper()
	u := &models.User{Email: email, CreatedAt: createdAt, LastLoginAt: lastLogin}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user %q failed: %v", email, err)
	}
	return u
}

func createTestWalletAccount(t *testing.T, db *gorm.DB, userID uint, balance int64) {
	t.Helper()
	w := &models.WalletAccount{UserID: userID, Balance: models.NewMoneyFromDecimal(decimal.NewFromInt(balance))}
	if err := db.Create(w).Error; err != nil {
		t.Fatalf("create wallet account for user %d failed: %v", userID, err)
	}
}

func timePtr(tm time.Time) *time.Time { return &tm }

func assertUserOrder(t *testing.T, rows []models.User, want []uint) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("expected %d rows, got %d", len(want), len(rows))
	}
	for i, id := range want {
		if rows[i].ID != id {
			gotIDs := make([]uint, len(rows))
			for j, r := range rows {
				gotIDs[j] = r.ID
			}
			t.Fatalf("order mismatch: want %v, got %v", want, gotIDs)
		}
	}
}

// 按注册时间升序：最早注册的排在最前
func TestUserRepositoryListSortByCreatedAtAsc(t *testing.T) {
	repo, db := setupUserRepositoryTest(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := createTestUser(t, db, "newest@x.com", base.Add(2*time.Hour), nil)
	middle := createTestUser(t, db, "middle@x.com", base.Add(1*time.Hour), nil)
	oldest := createTestUser(t, db, "oldest@x.com", base, nil)

	rows, total, err := repo.List(UserListFilter{SortBy: "created_at", SortOrder: "asc"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	assertUserOrder(t, rows, []uint{oldest.ID, middle.ID, newest.ID})
}

// 按最后登录时间降序：从未登录（NULL）的用户始终排在最后
func TestUserRepositoryListSortByLastLoginDescNullsLast(t *testing.T) {
	repo, db := setupUserRepositoryTest(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := createTestUser(t, db, "recent@x.com", base, timePtr(base.Add(2*time.Hour)))
	older := createTestUser(t, db, "older@x.com", base, timePtr(base.Add(1*time.Hour)))
	never := createTestUser(t, db, "never@x.com", base, nil)

	rows, _, err := repo.List(UserListFilter{SortBy: "last_login_at", SortOrder: "desc"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	assertUserOrder(t, rows, []uint{recent.ID, older.ID, never.ID})
}

// 按钱包余额降序：无钱包账户的用户按余额 0 处理并排在最后
func TestUserRepositoryListSortByWalletBalanceDesc(t *testing.T) {
	repo, db := setupUserRepositoryTest(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rich := createTestUser(t, db, "rich@x.com", base, nil)
	poor := createTestUser(t, db, "poor@x.com", base, nil)
	noAccount := createTestUser(t, db, "none@x.com", base, nil)
	createTestWalletAccount(t, db, rich.ID, 100)
	createTestWalletAccount(t, db, poor.ID, 10)

	rows, _, err := repo.List(UserListFilter{SortBy: "wallet_balance", SortOrder: "desc"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	assertUserOrder(t, rows, []uint{rich.ID, poor.ID, noAccount.ID})
}

// 非法 sort_by 回退到默认排序（id 倒序）
func TestUserRepositoryListSortByInvalidFallsBackToIDDesc(t *testing.T) {
	repo, db := setupUserRepositoryTest(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	u1 := createTestUser(t, db, "u1@x.com", base, nil)
	u2 := createTestUser(t, db, "u2@x.com", base, nil)
	u3 := createTestUser(t, db, "u3@x.com", base, nil)

	rows, _, err := repo.List(UserListFilter{SortBy: "drop table users", SortOrder: "desc"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	assertUserOrder(t, rows, []uint{u3.ID, u2.ID, u1.ID})
}
