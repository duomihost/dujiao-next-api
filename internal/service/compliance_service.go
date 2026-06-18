package service

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
)

const (
	complianceSettingKey = "compliance.acknowledgement.v1"
	complianceVersion    = "v1"

	expectedSegment1 = "我已阅读并理解上述合规声明提醒"
	expectedSegment2 = "知悉相关法律风险"
	expectedSegment3 = "并确认自行承担部署运营和收费行为产生的法律责任"

	expectedFullText = "我已阅读并理解上述合规声明提醒，知悉相关法律风险，并确认自行承担部署、运营和收费行为产生的法律责任"
)

var (
	ErrComplianceTextMismatch = errors.New("compliance text mismatch")
	ErrAlreadyAcknowledged    = errors.New("compliance already acknowledged")
)

// AcknowledgeRequest 确认请求
type AcknowledgeRequest struct {
	Segment1  string
	Segment2  string
	Segment3  string
	AdminID   uint
	Username  string
	ClientIP  string
	UserAgent string
}

// ComplianceStatus 状态响应
type ComplianceStatus struct {
	Acknowledged           bool   `json:"acknowledged"`
	AcknowledgedAt         string `json:"acknowledged_at,omitempty"`
	AcknowledgedByAdminID  uint   `json:"acknowledged_by_admin_id,omitempty"`
	AcknowledgedByUsername string `json:"acknowledged_by_username,omitempty"`
	Version                string `json:"version,omitempty"`
}

// ComplianceService 合规声明确认服务
type ComplianceService struct {
	settingRepo repository.SettingRepository
	acked       atomic.Bool
	writeMu     sync.Mutex // 保护 Acknowledge 的 check-then-act
}

// NewComplianceService 启动时装载一次 DB
func NewComplianceService(repo repository.SettingRepository) *ComplianceService {
	s := &ComplianceService{settingRepo: repo}
	if status, err := s.Status(); err == nil && status.Acknowledged {
		s.acked.Store(true)
	}
	return s
}

// IsAcknowledged 中间件级快速判定，不查库
func (s *ComplianceService) IsAcknowledged() bool {
	return s.acked.Load()
}

// Status 读取当前状态（管理面 UI 展示用）
func (s *ComplianceService) Status() (*ComplianceStatus, error) {
	setting, err := s.settingRepo.GetByKey(complianceSettingKey)
	if err != nil {
		return nil, fmt.Errorf("compliance: read setting: %w", err)
	}
	if setting == nil {
		return &ComplianceStatus{Acknowledged: false}, nil
	}
	v := setting.ValueJSON
	status := &ComplianceStatus{
		Acknowledged: complianceJSONBool(v, "acknowledged"),
		Version:      complianceJSONString(v, "version"),
	}
	status.AcknowledgedAt = complianceJSONString(v, "acknowledged_at")
	if id, ok := v["acknowledged_by_admin_id"].(float64); ok {
		status.AcknowledgedByAdminID = uint(id)
	}
	status.AcknowledgedByUsername = complianceJSONString(v, "acknowledged_by_username")
	return status, nil
}

// Acknowledge 写入确认；幂等保护：已确认返回 ErrAlreadyAcknowledged
func (s *ComplianceService) Acknowledge(req AcknowledgeRequest) error {
	if req.Segment1 != expectedSegment1 ||
		req.Segment2 != expectedSegment2 ||
		req.Segment3 != expectedSegment3 {
		return ErrComplianceTextMismatch
	}
	// 拼接二次校验：从调用方实际传入的 segments 重建完整文本（防御深度）
	// segment3 不含 、，故拆出 "部署"/"运营" 边界后插入
	const seg3SplitPrefix = "并确认自行承担部署"
	if !strings.HasPrefix(req.Segment3, seg3SplitPrefix) {
		return ErrComplianceTextMismatch
	}
	full := req.Segment1 + "，" + req.Segment2 + "，" + seg3SplitPrefix + "、" + req.Segment3[len(seg3SplitPrefix):]
	if full != expectedFullText {
		return ErrComplianceTextMismatch
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.acked.Load() {
		return ErrAlreadyAcknowledged
	}

	value := models.JSON{
		"acknowledged":             true,
		"acknowledged_at":          time.Now().UTC().Format(time.RFC3339),
		"acknowledged_by_admin_id": req.AdminID,
		"acknowledged_by_username": req.Username,
		"acknowledged_text":        expectedFullText,
		"version":                  complianceVersion,
		"client_ip":                req.ClientIP,
		"user_agent":               req.UserAgent,
	}
	if _, err := s.settingRepo.Upsert(complianceSettingKey, value); err != nil {
		return fmt.Errorf("compliance: upsert setting: %w", err)
	}
	s.acked.Store(true)
	return nil
}

func complianceJSONBool(j models.JSON, key string) bool {
	v, ok := j[key].(bool)
	return ok && v
}

func complianceJSONString(j models.JSON, key string) string {
	v, _ := j[key].(string)
	return v
}
