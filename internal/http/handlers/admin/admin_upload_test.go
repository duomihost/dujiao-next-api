package admin

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/provider"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

func TestUploadFileReturnsValidationMessageForOversizedFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "oversized.png")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write([]byte("oversized")); err != nil {
		t.Fatalf("write form file failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	cfg := &config.Config{}
	cfg.Upload.MaxSize = 4
	handler := &Handler{Container: &provider.Container{
		UploadService: service.NewUploadService(cfg),
	}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/upload", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	handler.UploadFile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("http status want 200 got %d body=%s", w.Code, w.Body.String())
	}
	var got response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, w.Body.String())
	}
	if got.StatusCode != response.CodeBadRequest {
		t.Fatalf("business status want 400 got %d body=%s", got.StatusCode, w.Body.String())
	}
	if !strings.Contains(got.Msg, "文件大小超过限制") {
		t.Fatalf("message should include validation reason, got %q", got.Msg)
	}
}
