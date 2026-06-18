package admin

import (
	"errors"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetResellerOperationsOverview(c *gin.Context) {
	input, err := parseDashboardQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	if h.ResellerOperationsService == nil {
		shared.RespondError(c, response.CodeInternal, "error.dashboard_fetch_failed", nil)
		return
	}
	data, err := h.ResellerOperationsService.GetOverview(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrDashboardRangeInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.dashboard_fetch_failed", err)
		return
	}
	response.Success(c, data)
}

func (h *Handler) GetResellerOperationsFinance(c *gin.Context) {
	input, err := parseDashboardQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	if h.ResellerOperationsService == nil {
		shared.RespondError(c, response.CodeInternal, "error.dashboard_fetch_failed", nil)
		return
	}
	data, err := h.ResellerOperationsService.GetFinance(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrDashboardRangeInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.dashboard_fetch_failed", err)
		return
	}
	response.Success(c, data)
}
