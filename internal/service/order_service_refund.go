package service

import (
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// AdminManualRefundInput 管理员手动退款输入（不处理钱包/支付渠道）
type AdminManualRefundInput struct {
	OrderID uint
	Amount  models.Money
	Remark  string
}

// AdminOrderRefundListQuery 管理端退款记录列表查询条件（原始输入）。
type AdminOrderRefundListQuery struct {
	Page           int
	PageSize       int
	UserID         string
	UserKeyword    string
	OrderNo        string
	GuestEmail     string
	ProductKeyword string
	ProductName    string
	CreatedFrom    string
	CreatedTo      string
}

// AdminOrderRefundItem 管理端订单退款返回项（列表/详情统一字段）。
type AdminOrderRefundItem struct {
	models.OrderRefundRecord
	OrderNo         string             `json:"order_no,omitempty"`
	GuestLocale     string             `json:"guest_locale,omitempty"`
	Items           []models.OrderItem `json:"items,omitempty"`
	UserEmail       string             `json:"user_email,omitempty"`
	UserDisplayName string             `json:"user_display_name,omitempty"`
	RefundTypeLabel string             `json:"refund_type_label"`
}

// OrderRefundService 订单退款服务（手动退款）
type OrderRefundService struct {
	orderRepo             repository.OrderRepository
	userRepo              repository.UserRepository
	orderRefundRecordRepo repository.OrderRefundRecordRepository
	affiliateSvc          *AffiliateService
	settingService        *SettingService
	resellerAccountingSvc *ResellerAccountingService
}

// OrderStatusEmailRefundDetails 订单状态邮件中的退款信息
type OrderStatusEmailRefundDetails struct {
	Amount models.Money
	Reason string
}

// ResolveOrderStatusEmailRefundDetails 解析订单状态邮件展示的退款信息。
// 优先使用 refund_record_id 对应记录；若无匹配或服务不可用，回退到订单累计退款金额。
// 当解析退款记录失败时，仍返回回退结果，并透传 error 供调用方记录日志。
func (s *OrderRefundService) ResolveOrderStatusEmailRefundDetails(order *models.Order, refundRecordID uint) (OrderStatusEmailRefundDetails, error) {
	if order == nil {
		return OrderStatusEmailRefundDetails{}, nil
	}
	fallback := OrderStatusEmailRefundDetails{
		Amount: order.RefundedAmount,
		Reason: "",
	}
	if s == nil || refundRecordID == 0 {
		return fallback, nil
	}
	details, ok, err := s.ResolveStatusEmailRefundDetails(order.ID, refundRecordID)
	if err != nil {
		return fallback, err
	}
	if ok {
		return details, nil
	}
	return fallback, nil
}

// NewOrderRefundService 创建订单退款服务
func NewOrderRefundService(
	orderRepo repository.OrderRepository,
	userRepo repository.UserRepository,
	orderRefundRecordRepo repository.OrderRefundRecordRepository,
	affiliateSvc *AffiliateService,
	settingService *SettingService,
) *OrderRefundService {
	return &OrderRefundService{
		orderRepo:             orderRepo,
		userRepo:              userRepo,
		orderRefundRecordRepo: orderRefundRecordRepo,
		affiliateSvc:          affiliateSvc,
		settingService:        settingService,
	}
}

func (s *OrderRefundService) SetResellerAccountingService(svc *ResellerAccountingService) {
	s.resellerAccountingSvc = svc
}

// ParseRefundAmount 解析并校验退款金额。
func (s *OrderRefundService) ParseRefundAmount(raw string) (models.Money, error) {
	parsed, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return models.Money{}, ErrWalletInvalidAmount
	}
	amount := parsed.Round(2)
	if amount.LessThanOrEqual(decimal.Zero) {
		return models.Money{}, ErrWalletInvalidAmount
	}
	return models.NewMoneyFromDecimal(amount), nil
}

// ParseAdminRefundListFilter 解析管理端退款记录列表筛选条件。
func (s *OrderRefundService) ParseAdminRefundListFilter(query AdminOrderRefundListQuery) (repository.OrderRefundRecordListFilter, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	userID, err := parseOptionalUint(query.UserID)
	if err != nil {
		return repository.OrderRefundRecordListFilter{}, err
	}
	createdFrom, err := parseRFC3339Nullable(query.CreatedFrom)
	if err != nil {
		return repository.OrderRefundRecordListFilter{}, err
	}
	createdTo, err := parseRFC3339Nullable(query.CreatedTo)
	if err != nil {
		return repository.OrderRefundRecordListFilter{}, err
	}

	productKeyword := strings.TrimSpace(query.ProductKeyword)
	if productKeyword == "" {
		productKeyword = strings.TrimSpace(query.ProductName)
	}

	return repository.OrderRefundRecordListFilter{
		Page:           page,
		PageSize:       pageSize,
		UserID:         userID,
		UserKeyword:    strings.TrimSpace(query.UserKeyword),
		OrderNo:        strings.TrimSpace(query.OrderNo),
		GuestEmail:     strings.TrimSpace(query.GuestEmail),
		ProductKeyword: productKeyword,
		CreatedFrom:    createdFrom,
		CreatedTo:      createdTo,
	}, nil
}

// ListAdminRefundItems 管理端退款记录列表（含展示所需关联字段）。
func (s *OrderRefundService) ListAdminRefundItems(query AdminOrderRefundListQuery) ([]AdminOrderRefundItem, int64, error) {
	filter, err := s.ParseAdminRefundListFilter(query)
	if err != nil {
		return nil, 0, err
	}
	records, total, err := s.ListAdminRefundRecords(filter)
	if err != nil {
		return nil, 0, err
	}

	orderMap, err := s.resolveRefundOrders(records)
	if err != nil {
		return nil, 0, ErrOrderFetchFailed
	}
	userMap, err := s.resolveRefundUsers(records, orderMap)
	if err != nil {
		return nil, 0, ErrOrderFetchFailed
	}

	items := make([]AdminOrderRefundItem, 0, len(records))
	for _, record := range records {
		order := orderMap[record.OrderID]
		items = append(items, s.buildAdminRefundItem(record, order, userMap))
	}
	return items, total, nil
}

// GetAdminRefundItem 管理端退款记录详情（含展示所需关联字段）。
func (s *OrderRefundService) GetAdminRefundItem(id uint) (*AdminOrderRefundItem, error) {
	record, err := s.GetAdminRefundRecord(id)
	if err != nil {
		return nil, err
	}
	if s == nil || s.orderRepo == nil {
		return nil, ErrOrderFetchFailed
	}

	order, err := s.orderRepo.GetByID(record.OrderID)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	userMap, err := s.resolveRefundUsers([]models.OrderRefundRecord{*record}, map[uint]*models.Order{
		record.OrderID: order,
	})
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	item := s.buildAdminRefundItem(*record, order, userMap)
	return &item, nil
}

// AdminManualRefund 管理端手动退款（不处理钱包/支付渠道，仅写 order_refund_records）
func (s *OrderRefundService) AdminManualRefund(input AdminManualRefundInput) (*models.Order, *models.OrderRefundRecord, error) {
	if input.OrderID == 0 {
		return nil, nil, ErrOrderNotFound
	}
	amount := input.Amount.Decimal.Round(2)
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, nil, ErrWalletInvalidAmount
	}
	recordRemark := strings.TrimSpace(input.Remark)
	var createdRecord *models.OrderRefundRecord

	cfg := DefaultOrderRefundConfig()
	if s.settingService != nil {
		cfgLoaded, cfgErr := s.settingService.GetOrderRefundConfig()
		if cfgErr != nil {
			return nil, nil, cfgErr
		}
		cfg = cfgLoaded
	}

	if err := s.orderRepo.Transaction(func(tx *gorm.DB) error {
		locked, err := s.orderRepo.WithTx(tx).GetByIDForUpdate(input.OrderID)
		if err != nil {
			return err
		}
		if locked == nil {
			return ErrOrderNotFound
		}
		order := *locked
		if order.PaidAt == nil {
			return ErrOrderStatusInvalid
		}
		if isOrderRefundWindowExpired(&order, cfg.MaxRefundDays, time.Now()) {
			return ErrOrderRefundExpired
		}
		if order.TotalAmount.Decimal.LessThanOrEqual(decimal.Zero) {
			return ErrOrderStatusInvalid
		}
		refundedBefore := order.RefundedAmount.Decimal.Round(2)
		refundable := order.TotalAmount.Decimal.Sub(refundedBefore).Round(2)
		if amount.GreaterThan(refundable) {
			return ErrWalletRefundExceeded
		}

		newRefunded := refundedBefore.Add(amount).Round(2)
		now := time.Now()
		updates := map[string]interface{}{
			"refunded_amount": models.NewMoneyFromDecimal(newRefunded),
			"updated_at":      now,
		}
		markRefunded := newRefunded.GreaterThanOrEqual(order.TotalAmount.Decimal.Round(2))
		if markRefunded {
			updates["status"] = constants.OrderStatusRefunded
		} else {
			updates["status"] = constants.OrderStatusPartiallyRefunded
		}
		if err := s.orderRepo.WithTx(tx).UpdateFields(order.ID, updates); err != nil {
			return ErrOrderUpdateFailed
		}
		if order.ParentID == nil {
			targetStatus := constants.OrderStatusPartiallyRefunded
			if markRefunded {
				targetStatus = constants.OrderStatusRefunded
			}
			if err := applyParentRefundChildStatusUpdates(s.orderRepo.WithTx(tx), order.ID, targetStatus, now); err != nil {
				return ErrOrderUpdateFailed
			}
		}
		if order.ParentID != nil {
			if _, err := syncParentStatus(s.orderRepo.WithTx(tx), *order.ParentID, now); err != nil {
				return ErrOrderUpdateFailed
			}
		}
		record, err := s.createRefundRecordTx(tx, &order, constants.OrderRefundTypeManual, amount, recordRemark, now)
		if err != nil {
			return err
		}
		createdRecord = record
		if s.affiliateSvc != nil && order.UserID > 0 {
			if err := s.affiliateSvc.HandleOrderRefundedTx(
				tx,
				&order,
				amount,
				refundedBefore,
				"order_refunded_manual",
			); err != nil {
				return err
			}
		}
		if s.resellerAccountingSvc != nil {
			if err := s.resellerAccountingSvc.HandleRefundDeductTx(tx, &order, record, refundedBefore); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}

	order, err := s.orderRepo.GetByID(input.OrderID)
	if err != nil {
		return nil, nil, ErrOrderFetchFailed
	}
	if order == nil {
		return nil, nil, ErrOrderNotFound
	}
	return order, createdRecord, nil
}

// ListAdminRefundRecords 管理端退款记录列表
func (s *OrderRefundService) ListAdminRefundRecords(filter repository.OrderRefundRecordListFilter) ([]models.OrderRefundRecord, int64, error) {
	if s == nil || s.orderRefundRecordRepo == nil {
		return nil, 0, ErrOrderFetchFailed
	}
	records, total, err := s.orderRefundRecordRepo.ListAdmin(filter)
	if err != nil {
		return nil, 0, ErrOrderFetchFailed
	}
	return records, total, nil
}

// GetAdminRefundRecord 管理端退款记录详情
func (s *OrderRefundService) GetAdminRefundRecord(id uint) (*models.OrderRefundRecord, error) {
	if s == nil || s.orderRefundRecordRepo == nil {
		return nil, ErrOrderFetchFailed
	}
	if id == 0 {
		return nil, ErrOrderNotFound
	}
	record, err := s.orderRefundRecordRepo.GetByID(id)
	if err != nil {
		return nil, ErrOrderFetchFailed
	}
	if record == nil {
		return nil, ErrOrderNotFound
	}
	return record, nil
}

// ResolveStatusEmailRefundDetails 按退款记录解析订单状态邮件所需的退款金额与原因。
// 仅当退款记录与目标订单有关联时返回 true（同一订单，或退款记录来自该父订单的子订单）。
func (s *OrderRefundService) ResolveStatusEmailRefundDetails(orderID uint, refundRecordID uint) (OrderStatusEmailRefundDetails, bool, error) {
	if s == nil || s.orderRefundRecordRepo == nil || refundRecordID == 0 {
		return OrderStatusEmailRefundDetails{}, false, nil
	}

	record, err := s.orderRefundRecordRepo.GetByID(refundRecordID)
	if err != nil {
		return OrderStatusEmailRefundDetails{}, false, ErrOrderFetchFailed
	}
	if record == nil {
		return OrderStatusEmailRefundDetails{}, false, nil
	}
	if orderID == 0 {
		return OrderStatusEmailRefundDetails{
			Amount: record.Amount,
			Reason: strings.TrimSpace(record.Remark),
		}, true, nil
	}
	if record.OrderID == orderID {
		return OrderStatusEmailRefundDetails{
			Amount: record.Amount,
			Reason: strings.TrimSpace(record.Remark),
		}, true, nil
	}
	if s.orderRepo == nil {
		return OrderStatusEmailRefundDetails{}, false, nil
	}

	recordOrder, err := s.orderRepo.GetByID(record.OrderID)
	if err != nil {
		return OrderStatusEmailRefundDetails{}, false, ErrOrderFetchFailed
	}
	if recordOrder != nil && recordOrder.ParentID != nil && *recordOrder.ParentID == orderID {
		return OrderStatusEmailRefundDetails{
			Amount: record.Amount,
			Reason: strings.TrimSpace(record.Remark),
		}, true, nil
	}
	return OrderStatusEmailRefundDetails{}, false, nil
}

// createRefundRecordTx 在事务内写入退款记录（order_refund_records）。
func (s *OrderRefundService) createRefundRecordTx(
	tx *gorm.DB,
	order *models.Order,
	refundType string,
	amount decimal.Decimal,
	remark string,
	now time.Time,
) (*models.OrderRefundRecord, error) {
	if tx == nil || order == nil || s.orderRefundRecordRepo == nil {
		return nil, ErrRefundRecordCreateFailed
	}
	record := &models.OrderRefundRecord{
		UserID:     order.UserID,
		GuestEmail: order.GuestEmail,
		OrderID:    order.ID,
		Type:       strings.TrimSpace(refundType),
		Amount:     models.NewMoneyFromDecimal(amount.Round(2)),
		Currency:   normalizeWalletCurrency(order.Currency),
		Remark:     remark,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.orderRefundRecordRepo.WithTx(tx).Create(record); err != nil {
		return nil, ErrRefundRecordCreateFailed
	}
	return record, nil
}

// parseOptionalUint 解析可选正整数查询参数，空串返回 0。
func parseOptionalUint(raw string) (uint, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(parsed), nil
}

// parseRFC3339Nullable 解析可空 RFC3339 时间，空串返回 nil。
func parseRFC3339Nullable(raw string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// resolveRefundOrders 批量加载退款记录关联订单，避免重复查询。
func (s *OrderRefundService) resolveRefundOrders(records []models.OrderRefundRecord) (map[uint]*models.Order, error) {
	result := make(map[uint]*models.Order, len(records))
	if s == nil || s.orderRepo == nil {
		return result, ErrOrderFetchFailed
	}
	seen := make(map[uint]struct{}, len(records))
	for _, record := range records {
		if record.OrderID == 0 {
			continue
		}
		if _, ok := seen[record.OrderID]; ok {
			continue
		}
		seen[record.OrderID] = struct{}{}
		order, err := s.orderRepo.GetByID(record.OrderID)
		if err != nil {
			return nil, err
		}
		result[record.OrderID] = order
	}
	return result, nil
}

// resolveRefundUsers 批量加载退款记录关联用户（游客退款不加载用户）。
func (s *OrderRefundService) resolveRefundUsers(records []models.OrderRefundRecord, orderMap map[uint]*models.Order) (map[uint]models.User, error) {
	result := make(map[uint]models.User)
	if s == nil || s.userRepo == nil {
		return result, nil
	}

	userIDs := make([]uint, 0, len(records))
	seen := make(map[uint]struct{}, len(records))
	for _, record := range records {
		userID := resolveRefundUserID(record, orderMap[record.OrderID])
		if userID == 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}
	if len(userIDs) == 0 {
		return result, nil
	}

	users, err := s.userRepo.ListByIDs(userIDs)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		result[user.ID] = user
	}
	return result, nil
}

// buildAdminRefundItem 组装管理端退款记录展示项。
func (s *OrderRefundService) buildAdminRefundItem(record models.OrderRefundRecord, order *models.Order, userMap map[uint]models.User) AdminOrderRefundItem {
	recordGuestEmail := resolveRefundGuestEmail(record, order)
	recordGuestLocale := resolveRefundGuestLocale(order)
	recordForResponse := record
	recordForResponse.GuestEmail = recordGuestEmail

	user := userMap[resolveRefundUserID(recordForResponse, order)]
	userEmail := strings.TrimSpace(user.Email)
	userDisplayName := strings.TrimSpace(user.DisplayName)
	if recordGuestEmail != "" {
		userEmail = ""
		userDisplayName = ""
	}

	return AdminOrderRefundItem{
		OrderRefundRecord: recordForResponse,
		OrderNo:           resolveRefundOrderNo(order),
		GuestLocale:       recordGuestLocale,
		Items:             collectRefundItems(order),
		UserEmail:         userEmail,
		UserDisplayName:   userDisplayName,
		RefundTypeLabel:   recordForResponse.Type,
	}
}

// resolveRefundUserID 解析退款记录对应的用户ID（游客订单返回0）。
func resolveRefundUserID(record models.OrderRefundRecord, order *models.Order) uint {
	if resolveRefundGuestEmail(record, order) != "" {
		return 0
	}
	if record.UserID > 0 {
		return record.UserID
	}
	if order != nil && order.UserID > 0 {
		return order.UserID
	}
	return 0
}

// resolveRefundGuestEmail 解析退款记录对应的游客邮箱（优先记录字段，其次订单字段）。
func resolveRefundGuestEmail(record models.OrderRefundRecord, order *models.Order) string {
	if guestEmail := strings.TrimSpace(record.GuestEmail); guestEmail != "" {
		return guestEmail
	}
	if order != nil {
		return strings.TrimSpace(order.GuestEmail)
	}
	return ""
}

// resolveRefundGuestLocale 解析退款订单的游客语言设置。
func resolveRefundGuestLocale(order *models.Order) string {
	if order == nil {
		return ""
	}
	return strings.TrimSpace(order.GuestLocale)
}

// resolveRefundOrderNo 解析退款记录关联订单号。
func resolveRefundOrderNo(order *models.Order) string {
	if order == nil {
		return ""
	}
	return strings.TrimSpace(order.OrderNo)
}

// collectRefundItems 收集退款记录对应订单及其子订单的商品项快照。
func collectRefundItems(order *models.Order) []models.OrderItem {
	if order == nil {
		return nil
	}
	result := make([]models.OrderItem, 0, len(order.Items))
	result = append(result, order.Items...)
	for i := range order.Children {
		result = append(result, order.Children[i].Items...)
	}
	return result
}
