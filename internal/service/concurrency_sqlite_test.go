package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dujiao-next/internal/models"

	"github.com/shopspring/decimal"
)

// TestSQLiteConcurrentTransactionsDoNotDeadlock 验证 P0 整改后,多个 goroutine
// 并发触发 service 事务方法不会因 service 内残留的非 WithTx 调用或事务内 HTTP/RPC
// 而死锁。
//
// 守护逻辑:把 SQLite 连接池压成 MaxOpenConns=1(生产配置),N 个 goroutine 并发
// 调用 WalletService.Recharge(各自不同 userID),5 秒内必须全部返回。
//
// 在 P0 整改之前(service 直接 tx.Model + 事务内可能调外部 service),如果某个
// service 方法在事务回调内通过非 tx 路径请求第二个 DB 连接,这个测试会超时挂死;
// 整改后所有 DB 写都走 repo.WithTx(tx),整个事务用单连接完成,并发只是排队不死锁。
func TestSQLiteConcurrentTransactionsDoNotDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in short mode")
	}

	svc, db := setupWalletServiceTest(t)

	// 模拟生产 SQLite 单写连接配置(CLAUDE.md:database.pool.max_open_conns=1)。
	// 这样如果有 service 在事务回调内再请求一个连接,会立刻阻塞触发死锁。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get raw db handle failed: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	const N = 8

	// 预先建好 N 个用户(GetOrCreateAccount 内部也走事务,但跟测试目标无关,
	// 这里 setup 阶段单线程跑完,排除噪音)。
	for i := 1; i <= N; i++ {
		createTestUser(t, db, uint(i))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, N)
	done := make(chan struct{})

	for i := 1; i <= N; i++ {
		wg.Add(1)
		go func(userID uint) {
			defer wg.Done()
			_, _, recErr := svc.Recharge(WalletRechargeInput{
				UserID:   userID,
				Amount:   models.NewMoneyFromDecimal(decimal.NewFromInt(100)),
				Currency: "CNY",
				Remark:   fmt.Sprintf("concurrency_smoke_%d", userID),
			})
			if recErr != nil {
				errCh <- fmt.Errorf("userID=%d: %w", userID, recErr)
			}
		}(uint(i))
	}

	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		// 所有 goroutine 在超时前返回,无死锁
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Recharge deadlocked (MaxOpenConns=1, 5s timeout exceeded)")
	}

	close(errCh)
	for e := range errCh {
		t.Errorf("recharge failure: %v", e)
	}

	// 验证每个用户余额都是 100(说明所有事务都真的 commit 了,不是被超时中断)
	for i := 1; i <= N; i++ {
		account, err := svc.GetAccount(uint(i))
		if err != nil {
			t.Fatalf("get account userID=%d failed: %v", i, err)
		}
		got := account.Balance.Decimal.Round(2)
		want := decimal.NewFromInt(100)
		if !got.Equal(want) {
			t.Errorf("userID=%d balance got %s, want %s", i, got, want)
		}
	}
}
