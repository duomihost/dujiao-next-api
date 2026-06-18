package worker

import (
	"context"
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/provider"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestResellerConfirmLedgerWorkerMarksDueEntriesAvailable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:reseller_worker?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	if err := db.AutoMigrate(&models.ResellerLedgerEntry{}, &models.ResellerBalanceAccount{}); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	past := time.Now().Add(-time.Minute)
	row := models.ResellerLedgerEntry{
		ResellerID:     1,
		Type:           models.ResellerLedgerTypeOrderProfit,
		Amount:         models.NewMoneyFromDecimal(decimal.NewFromInt(10)),
		Currency:       "USD",
		IdempotencyKey: "order_profit:worker",
		Status:         models.ResellerLedgerStatusPendingConfirm,
		AvailableAt:    &past,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed ledger failed: %v", err)
	}
	repo := repository.NewResellerRepository(db)
	c := NewConsumer(&provider.Container{
		ResellerAccountingService: service.NewResellerAccountingService(repo, service.ResellerAccountingOptions{ConfirmDays: 0}),
	})
	if err := c.handleResellerConfirmLedger(context.Background(), queue.NewResellerConfirmLedgerTask()); err != nil {
		t.Fatalf("worker handler failed: %v", err)
	}
	var got models.ResellerLedgerEntry
	if err := db.First(&got, row.ID).Error; err != nil {
		t.Fatalf("load ledger failed: %v", err)
	}
	if got.Status != models.ResellerLedgerStatusAvailable {
		t.Fatalf("expected available, got %s", got.Status)
	}
}

func TestResellerConfirmLedgerTaskType(t *testing.T) {
	task := queue.NewResellerConfirmLedgerTask()
	if task == nil {
		t.Fatal("expected task")
	}
	if task.Type() != queue.TaskResellerConfirmLedger {
		t.Fatalf("unexpected task type %s", task.Type())
	}
}

func TestResellerConfirmLedgerWorkerSkipNilService(t *testing.T) {
	c := NewConsumer(&provider.Container{})
	if err := c.handleResellerConfirmLedger(context.Background(), asynq.NewTask(queue.TaskResellerConfirmLedger, nil)); err != nil {
		t.Fatalf("nil service should be skipped, got %v", err)
	}
}
