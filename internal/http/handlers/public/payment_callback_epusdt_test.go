package public

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestEpusdtCallback_FeatureGuard_BodyWithoutPidFallsThrough
// Body 是 BEpusdt 风格（无 pid）→ HandleEpusdtCallback 应返回 false 让链向后传
func TestEpusdtCallback_FeatureGuard_BodyWithoutPidFallsThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}

	body := `{"trade_id":"t1","order_id":"o1","status":2,"signature":"x"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/payments/callback", bytes.NewBufferString(body))

	if handled := h.HandleEpusdtCallback(c); handled {
		t.Fatalf("expected handled=false (no pid), got true")
	}
}

// TestEpusdtCallback_FeatureGuard_EmptyBodyFallsThrough
// 空 body → 返回 false（链应继续）
func TestEpusdtCallback_FeatureGuard_EmptyBodyFallsThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/payments/callback", strings.NewReader(""))

	if handled := h.HandleEpusdtCallback(c); handled {
		t.Fatalf("expected handled=false for empty body")
	}
}
