package repository

import (
	"errors"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FulfillmentRepository 交付数据访问接口
type FulfillmentRepository interface {
	Create(fulfillment *models.Fulfillment) error
	GetByOrderID(orderID uint) (*models.Fulfillment, error)
	// FindByOrderIDForUpdate 在事务中查询交付记录,返回 (record, found, err)
	FindByOrderIDForUpdate(orderID uint) (*models.Fulfillment, bool, error)
	WithTx(tx *gorm.DB) *GormFulfillmentRepository
}

// GormFulfillmentRepository GORM 实现
type GormFulfillmentRepository struct {
	BaseRepository
}

// NewFulfillmentRepository 创建交付仓库
func NewFulfillmentRepository(db *gorm.DB) *GormFulfillmentRepository {
	return &GormFulfillmentRepository{BaseRepository: BaseRepository{db: db}}
}

// WithTx 绑定事务
func (r *GormFulfillmentRepository) WithTx(tx *gorm.DB) *GormFulfillmentRepository {
	if tx == nil {
		return r
	}
	return &GormFulfillmentRepository{BaseRepository: BaseRepository{db: tx}}
}

// Create 创建交付记录
func (r *GormFulfillmentRepository) Create(fulfillment *models.Fulfillment) error {
	return r.db.Create(fulfillment).Error
}

// GetByOrderID 根据订单 ID 获取交付记录(不存在返回 nil, nil),不加锁。
func (r *GormFulfillmentRepository) GetByOrderID(orderID uint) (*models.Fulfillment, error) {
	var existing models.Fulfillment
	if err := r.db.Where("order_id = ?", orderID).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &existing, nil
}

// FindByOrderIDForUpdate 用于事务内的存在性检查,加 SELECT ... FOR UPDATE 行锁防止并发双重交付。
// 返回 (record, found, err)。
func (r *GormFulfillmentRepository) FindByOrderIDForUpdate(orderID uint) (*models.Fulfillment, bool, error) {
	var existing models.Fulfillment
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ?", orderID).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &existing, true, nil
}
