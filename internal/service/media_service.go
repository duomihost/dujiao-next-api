package service

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
)

// MediaService 素材管理服务
type MediaService struct {
	repo repository.MediaRepository
}

// NewMediaService 创建素材服务实例
func NewMediaService(repo repository.MediaRepository) *MediaService {
	return &MediaService{repo: repo}
}

// List 素材列表
func (s *MediaService) List(scene, search string, page, pageSize int) ([]models.Media, int64, error) {
	return s.repo.List(repository.MediaListFilter{
		Page:     page,
		PageSize: pageSize,
		Scene:    scene,
		Search:   search,
	})
}

// RecordMedia 记录上传的素材元数据（上传后自动调用）
func (s *MediaService) RecordMedia(result *UploadResult, scene string) (*models.Media, error) {
	// 检查是否已存在（基于路径去重）
	existing, err := s.repo.GetByPath(result.URL)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	// 从原始文件名生成默认素材名称（去掉扩展名）
	name := result.Filename
	if idx := strings.LastIndex(name, "."); idx > 0 {
		name = name[:idx]
	}

	media := &models.Media{
		Name:     name,
		Filename: result.Filename,
		Path:     result.URL,
		MimeType: result.MimeType,
		Size:     result.Size,
		Scene:    scene,
		Width:    result.Width,
		Height:   result.Height,
	}
	if err := s.repo.Create(media); err != nil {
		return nil, err
	}
	return media, nil
}

// RecordLocalFile 将本地已存在的文件记录到素材库（用于下载的上游图片等）
// localPath 格式如 /uploads/upstream/uuid.jpg，scene 如 "upstream"
func (s *MediaService) RecordLocalFile(localPath, scene string) {
	// 去重
	existing, _ := s.repo.GetByPath(localPath)
	if existing != nil {
		return
	}

	// 本地物理路径：去掉开头的 /
	diskPath := strings.TrimPrefix(localPath, "/")
	fi, err := os.Stat(diskPath)
	if err != nil {
		return
	}

	filename := filepath.Base(localPath)
	name := filename
	if idx := strings.LastIndex(name, "."); idx > 0 {
		name = name[:idx]
	}

	// 检测 MIME 类型
	mimeType := "application/octet-stream"
	if f, err := os.Open(diskPath); err == nil {
		buf := make([]byte, 512)
		if n, _ := f.Read(buf); n > 0 {
			mimeType = http.DetectContentType(buf[:n])
		}
		f.Close()
	}

	// 尝试获取图片尺寸
	var width, height int
	if strings.HasPrefix(mimeType, "image/") && mimeType != "image/svg+xml" {
		if f, err := os.Open(diskPath); err == nil {
			if cfg, _, err := image.DecodeConfig(f); err == nil {
				width = cfg.Width
				height = cfg.Height
			}
			f.Close()
		}
	}

	media := &models.Media{
		Name:     name,
		Filename: filename,
		Path:     localPath,
		MimeType: mimeType,
		Size:     fi.Size(),
		Scene:    scene,
		Width:    width,
		Height:   height,
	}
	if err := s.repo.Create(media); err != nil {
		logger.Warnw("media_record_local_file_failed", "path", localPath, "error", err)
	}
}

// Rename 重命名素材
func (s *MediaService) Rename(id uint, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrMediaNameEmpty
	}
	media, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if media == nil {
		return ErrMediaNotFound
	}
	media.Name = name
	return s.repo.Update(media)
}

// Delete 删除素材（软删除记录并删除物理文件）
func (s *MediaService) Delete(id uint) error {
	media, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if media == nil {
		return ErrMediaNotFound
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	// 删除物理文件（Path 格式如 /uploads/product/2026/04/uuid.jpg）
	diskPath := strings.TrimPrefix(media.Path, "/")
	if err := os.Remove(diskPath); err != nil && !os.IsNotExist(err) {
		logger.Warnw("media_delete_file_failed", "id", id, "path", diskPath, "error", err)
	}
	return nil
}

// BatchDelete 批量删除素材，复用单个删除逻辑以保持文件清理行为一致。
func (s *MediaService) BatchDelete(ids []uint) (int, []uint) {
	successCount := 0
	failedIDs := make([]uint, 0)
	for _, id := range ids {
		if err := s.Delete(id); err == nil {
			successCount++
		} else {
			failedIDs = append(failedIDs, id)
		}
	}
	return successCount, failedIDs
}
