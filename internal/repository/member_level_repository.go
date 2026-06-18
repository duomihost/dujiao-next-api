package repository

import (
	"errors"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// MemberLevelRepository 会员等级数据访问接口
type MemberLevelRepository interface {
	GetByID(id uint) (*models.MemberLevel, error)
	GetBySlug(slug string) (*models.MemberLevel, error)
	GetDefault() (*models.MemberLevel, error)
	GetActiveBySortOrder(sortOrder int, excludeID uint) (*models.MemberLevel, error)
	ListAllActive() ([]models.MemberLevel, error)
	Create(level *models.MemberLevel) error
	Update(level *models.MemberLevel) error
	Delete(id uint) error
	List(filter MemberLevelListFilter) ([]models.MemberLevel, int64, error)
	ClearDefault(excludeID uint) error
	WithTx(tx *gorm.DB) *GormMemberLevelRepository
}

// MemberLevelListFilter 等级列表筛选
type MemberLevelListFilter struct {
	IsActive *bool
	Page     int
	PageSize int
}

// GormMemberLevelRepository GORM 实现
type GormMemberLevelRepository struct {
	db *gorm.DB
}

// NewMemberLevelRepository 创建会员等级仓库
func NewMemberLevelRepository(db *gorm.DB) *GormMemberLevelRepository {
	return &GormMemberLevelRepository{db: db}
}

func (r *GormMemberLevelRepository) WithTx(tx *gorm.DB) *GormMemberLevelRepository {
	if tx == nil {
		return r
	}
	return &GormMemberLevelRepository{db: tx}
}

func (r *GormMemberLevelRepository) GetByID(id uint) (*models.MemberLevel, error) {
	var level models.MemberLevel
	if err := r.db.First(&level, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &level, nil
}

func (r *GormMemberLevelRepository) GetBySlug(slug string) (*models.MemberLevel, error) {
	var level models.MemberLevel
	if err := r.db.Where("slug = ?", slug).First(&level).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &level, nil
}

func (r *GormMemberLevelRepository) GetDefault() (*models.MemberLevel, error) {
	var level models.MemberLevel
	if err := r.db.Where("is_default = ? AND is_active = ?", true, true).First(&level).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &level, nil
}

func (r *GormMemberLevelRepository) GetActiveBySortOrder(sortOrder int, excludeID uint) (*models.MemberLevel, error) {
	var level models.MemberLevel
	query := r.db.Where("is_active = ? AND sort_order = ?", true, sortOrder)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	if err := query.First(&level).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &level, nil
}

// ListAllActive 获取所有启用的等级，按 sort_order DESC
func (r *GormMemberLevelRepository) ListAllActive() ([]models.MemberLevel, error) {
	var levels []models.MemberLevel
	if err := r.db.Where("is_active = ?", true).Order("sort_order desc, id asc").Find(&levels).Error; err != nil {
		return nil, err
	}
	return levels, nil
}

func (r *GormMemberLevelRepository) Create(level *models.MemberLevel) error {
	return r.db.Create(level).Error
}

func (r *GormMemberLevelRepository) Update(level *models.MemberLevel) error {
	return r.db.Save(level).Error
}

func (r *GormMemberLevelRepository) Delete(id uint) error {
	return r.db.Delete(&models.MemberLevel{}, id).Error
}

func (r *GormMemberLevelRepository) List(filter MemberLevelListFilter) ([]models.MemberLevel, int64, error) {
	var levels []models.MemberLevel
	query := r.db.Model(&models.MemberLevel{})

	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = applyPagination(query, filter.Page, filter.PageSize)

	if err := query.Order("sort_order desc, id asc").Find(&levels).Error; err != nil {
		return nil, 0, err
	}
	return levels, total, nil
}

// ClearDefault 清除默认标记（排除指定ID）
func (r *GormMemberLevelRepository) ClearDefault(excludeID uint) error {
	query := r.db.Model(&models.MemberLevel{}).Where("is_default = ?", true)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	return query.Update("is_default", false).Error
}
