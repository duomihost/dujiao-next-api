package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/provider"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
)

type adminResellerOperationsRepoStub struct {
	overview repository.ResellerOperationsOverviewRow
	finance  repository.ResellerOperationsFinanceRowSet
}

func (s adminResellerOperationsRepoStub) GetOverview(startAt, endAt time.Time) (repository.ResellerOperationsOverviewRow, error) {
	return s.overview, nil
}

func (s adminResellerOperationsRepoStub) GetFinance(startAt, endAt time.Time) (repository.ResellerOperationsFinanceRowSet, error) {
	return s.finance, nil
}

func TestAdminResellerOperationsOverview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := New(&provider.Container{
		ResellerOperationsService: service.NewResellerOperationsService(adminResellerOperationsRepoStub{
			overview: repository.ResellerOperationsOverviewRow{
				Lifecycle: repository.ResellerOperationsLifecycleRow{ProfilesTotal: 2, ProfilesActive: 1},
				Orders:    repository.ResellerOperationsOrdersRow{OrdersTotal: 3, PaidOrders: 2},
			},
		}),
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/resellers/operations/overview?range=today&tz=Asia/Shanghai", nil)

	h.GetResellerOperationsOverview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("http status want 200 got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			Lifecycle struct {
				ProfilesTotal int64 `json:"profiles_total"`
			} `json:"lifecycle"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.StatusCode != response.CodeOK || resp.Data.Lifecycle.ProfilesTotal != 2 {
		t.Fatalf("unexpected payload: %+v", resp)
	}
}

func TestAdminResellerOperationsFinanceRejectsInvalidRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := New(&provider.Container{
		ResellerOperationsService: service.NewResellerOperationsService(adminResellerOperationsRepoStub{}),
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/resellers/operations/finance?range=custom", nil)

	h.GetResellerOperationsFinance(c)

	if w.Code != http.StatusOK {
		t.Fatalf("http status want 200 got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.StatusCode != response.CodeBadRequest {
		t.Fatalf("business status want 400 got %d body=%s", resp.StatusCode, w.Body.String())
	}
}

var _ repository.ResellerOperationsRepository = adminResellerOperationsRepoStub{}
