package service

import (
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/repository"
)

// applyParentRefundChildStatusUpdates 根据父订单退款结果统一更新子订单状态。
// 规则:子订单退款状态始终跟随父订单,避免分支逻辑造成状态混乱。
//
// orderRepo 必须是已绑定事务的实例(调用方传入 s.orderRepo.WithTx(tx)),
// 这样 service 层不再直接持有 *gorm.DB。
func applyParentRefundChildStatusUpdates(orderRepo repository.OrderRepository, parentOrderID uint, parentTargetStatus string, now time.Time) error {
	if orderRepo == nil || parentOrderID == 0 {
		return nil
	}

	target := strings.ToLower(strings.TrimSpace(parentTargetStatus))
	if target != constants.OrderStatusPartiallyRefunded && target != constants.OrderStatusRefunded {
		return nil
	}

	affected, err := orderRepo.UpdateChildrenStatus(parentOrderID, target, now)
	if err != nil {
		return err
	}
	if affected > 0 {
		logger.Debugw("refund_child_status_propagated",
			"parent_order_id", parentOrderID,
			"target_status", target,
			"rows_affected", affected,
		)
	}
	return nil
}
