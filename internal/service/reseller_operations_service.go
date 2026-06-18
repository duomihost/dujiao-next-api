package service

import (
	"context"
	"strings"
	"time"

	"github.com/dujiao-next/internal/repository"
	"github.com/shopspring/decimal"
)

type ResellerOperationsService struct {
	repo repository.ResellerOperationsRepository
}

func NewResellerOperationsService(repo repository.ResellerOperationsRepository) *ResellerOperationsService {
	return &ResellerOperationsService{repo: repo}
}

type ResellerOperationsOverviewResponse struct {
	Range        string                                  `json:"range"`
	From         string                                  `json:"from"`
	To           string                                  `json:"to"`
	Timezone     string                                  `json:"timezone"`
	Lifecycle    ResellerOperationsLifecycleResponse     `json:"lifecycle"`
	Orders       ResellerOperationsOrdersResponse        `json:"orders"`
	TopResellers []ResellerOperationsTopResellerResponse `json:"top_resellers"`
	Alerts       []ResellerOperationsAlertItem           `json:"alerts"`
}

type ResellerOperationsLifecycleResponse struct {
	ProfilesTotal                   int64 `json:"profiles_total"`
	ProfilesPendingReview           int64 `json:"profiles_pending_review"`
	ProfilesActive                  int64 `json:"profiles_active"`
	ProfilesRejected                int64 `json:"profiles_rejected"`
	ProfilesDisabled                int64 `json:"profiles_disabled"`
	ProfilesSettlementFrozen        int64 `json:"profiles_settlement_frozen"`
	DomainsTotal                    int64 `json:"domains_total"`
	DomainsPendingReview            int64 `json:"domains_pending_review"`
	DomainsActive                   int64 `json:"domains_active"`
	DomainsDisabled                 int64 `json:"domains_disabled"`
	DomainsPendingVerification      int64 `json:"domains_pending_verification"`
	DomainsVerified                 int64 `json:"domains_verified"`
	CustomDomains                   int64 `json:"custom_domains"`
	Subdomains                      int64 `json:"subdomains"`
	SiteConfigsTotal                int64 `json:"site_configs_total"`
	ActiveProfilesWithoutSiteConfig int64 `json:"active_profiles_without_site_config"`
}

type ResellerOperationsOrdersResponse struct {
	OrdersTotal                        int64  `json:"orders_total"`
	PaidOrders                         int64  `json:"paid_orders"`
	CompletedOrders                    int64  `json:"completed_orders"`
	RefundedOrders                     int64  `json:"refunded_orders"`
	SelfDealingBlockedOrders           int64  `json:"self_dealing_blocked_orders"`
	ActiveResellersWithOrders          int64  `json:"active_resellers_with_orders"`
	AveragePaidOrdersPerActiveReseller string `json:"average_paid_orders_per_active_reseller"`
}

type ResellerOperationsTopResellerResponse struct {
	ResellerID     uint   `json:"reseller_id"`
	UserID         uint   `json:"user_id"`
	Email          string `json:"email,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	OrdersTotal    int64  `json:"orders_total"`
	PaidOrders     int64  `json:"paid_orders"`
	ActiveDomains  int64  `json:"active_domains"`
	SiteConfigured bool   `json:"site_configured"`
	LastOrderAt    string `json:"last_order_at,omitempty"`
}

type ResellerOperationsAlertItem struct {
	Type  string `json:"type"`
	Level string `json:"level"`
	Value int64  `json:"value"`
}

type ResellerOperationsFinanceResponse struct {
	Range               string                                      `json:"range"`
	From                string                                      `json:"from"`
	To                  string                                      `json:"to"`
	Timezone            string                                      `json:"timezone"`
	PeriodCurrencyRows  []ResellerOperationsPeriodCurrencyResponse  `json:"period_currency_rows"`
	CurrentCurrencyRows []ResellerOperationsCurrentCurrencyResponse `json:"current_currency_rows"`
}

type ResellerOperationsPeriodCurrencyResponse struct {
	Currency       string `json:"currency"`
	OrdersTotal    int64  `json:"orders_total"`
	PaidOrders     int64  `json:"paid_orders"`
	GMVPaid        string `json:"gmv_paid"`
	ProfitEarned   string `json:"profit_earned"`
	RefundDeducted string `json:"refund_deducted"`
	WithdrawPaid   string `json:"withdraw_paid"`
}

type ResellerOperationsCurrentCurrencyResponse struct {
	Currency                string `json:"currency"`
	AvailableBalance        string `json:"available_balance"`
	LockedBalance           string `json:"locked_balance"`
	NegativeBalance         string `json:"negative_balance"`
	PendingWithdrawCount    int64  `json:"pending_withdraw_count"`
	PendingWithdrawAmount   string `json:"pending_withdraw_amount"`
	NegativeBalanceAccounts int64  `json:"negative_balance_accounts"`
	FrozenBalanceAccounts   int64  `json:"frozen_balance_accounts"`
}

func (s *ResellerOperationsService) GetOverview(_ context.Context, input DashboardQueryInput) (*ResellerOperationsOverviewResponse, error) {
	window, err := resolveDashboardWindow(input, time.Now())
	if err != nil {
		return nil, err
	}
	resp := emptyResellerOperationsOverviewResponse(window)
	if s == nil || s.repo == nil {
		return resp, nil
	}
	row, err := s.repo.GetOverview(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}
	resp.Lifecycle = mapResellerOperationsLifecycle(row.Lifecycle)
	resp.Orders = mapResellerOperationsOrders(row.Orders)
	resp.TopResellers = mapResellerOperationsTopResellers(row.TopResellers)
	resp.Alerts = buildResellerOperationsAlerts(row)
	return resp, nil
}

func (s *ResellerOperationsService) GetFinance(_ context.Context, input DashboardQueryInput) (*ResellerOperationsFinanceResponse, error) {
	window, err := resolveDashboardWindow(input, time.Now())
	if err != nil {
		return nil, err
	}
	resp := emptyResellerOperationsFinanceResponse(window)
	if s == nil || s.repo == nil {
		return resp, nil
	}
	rows, err := s.repo.GetFinance(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}
	resp.PeriodCurrencyRows = make([]ResellerOperationsPeriodCurrencyResponse, 0, len(rows.PeriodCurrencyRows))
	for _, row := range rows.PeriodCurrencyRows {
		resp.PeriodCurrencyRows = append(resp.PeriodCurrencyRows, ResellerOperationsPeriodCurrencyResponse{
			Currency:       normalizeResellerOperationsCurrency(row.Currency),
			OrdersTotal:    row.OrdersTotal,
			PaidOrders:     row.PaidOrders,
			GMVPaid:        formatResellerOperationsDecimal(row.GMVPaid),
			ProfitEarned:   formatResellerOperationsDecimal(row.ProfitEarned),
			RefundDeducted: formatResellerOperationsDecimal(row.RefundDeducted),
			WithdrawPaid:   formatResellerOperationsDecimal(row.WithdrawPaid),
		})
	}
	resp.CurrentCurrencyRows = make([]ResellerOperationsCurrentCurrencyResponse, 0, len(rows.CurrentCurrencyRows))
	for _, row := range rows.CurrentCurrencyRows {
		resp.CurrentCurrencyRows = append(resp.CurrentCurrencyRows, ResellerOperationsCurrentCurrencyResponse{
			Currency:                normalizeResellerOperationsCurrency(row.Currency),
			AvailableBalance:        formatResellerOperationsDecimal(row.AvailableBalance),
			LockedBalance:           formatResellerOperationsDecimal(row.LockedBalance),
			NegativeBalance:         formatResellerOperationsDecimal(row.NegativeBalance),
			PendingWithdrawCount:    row.PendingWithdrawCount,
			PendingWithdrawAmount:   formatResellerOperationsDecimal(row.PendingWithdrawAmount),
			NegativeBalanceAccounts: row.NegativeBalanceAccounts,
			FrozenBalanceAccounts:   row.FrozenBalanceAccounts,
		})
	}
	return resp, nil
}

func emptyResellerOperationsOverviewResponse(window dashboardWindow) *ResellerOperationsOverviewResponse {
	return &ResellerOperationsOverviewResponse{
		Range:        window.rangeKey,
		From:         window.startAt.Format(time.RFC3339),
		To:           window.endAt.Add(-time.Second).Format(time.RFC3339),
		Timezone:     window.timezone,
		TopResellers: []ResellerOperationsTopResellerResponse{},
		Alerts:       []ResellerOperationsAlertItem{},
	}
}

func emptyResellerOperationsFinanceResponse(window dashboardWindow) *ResellerOperationsFinanceResponse {
	return &ResellerOperationsFinanceResponse{
		Range:               window.rangeKey,
		From:                window.startAt.Format(time.RFC3339),
		To:                  window.endAt.Add(-time.Second).Format(time.RFC3339),
		Timezone:            window.timezone,
		PeriodCurrencyRows:  []ResellerOperationsPeriodCurrencyResponse{},
		CurrentCurrencyRows: []ResellerOperationsCurrentCurrencyResponse{},
	}
}

func mapResellerOperationsLifecycle(row repository.ResellerOperationsLifecycleRow) ResellerOperationsLifecycleResponse {
	return ResellerOperationsLifecycleResponse{
		ProfilesTotal:                   row.ProfilesTotal,
		ProfilesPendingReview:           row.ProfilesPendingReview,
		ProfilesActive:                  row.ProfilesActive,
		ProfilesRejected:                row.ProfilesRejected,
		ProfilesDisabled:                row.ProfilesDisabled,
		ProfilesSettlementFrozen:        row.ProfilesSettlementFrozen,
		DomainsTotal:                    row.DomainsTotal,
		DomainsPendingReview:            row.DomainsPendingReview,
		DomainsActive:                   row.DomainsActive,
		DomainsDisabled:                 row.DomainsDisabled,
		DomainsPendingVerification:      row.DomainsPendingVerification,
		DomainsVerified:                 row.DomainsVerified,
		CustomDomains:                   row.CustomDomains,
		Subdomains:                      row.Subdomains,
		SiteConfigsTotal:                row.SiteConfigsTotal,
		ActiveProfilesWithoutSiteConfig: row.ActiveProfilesWithoutSiteConfig,
	}
}

func mapResellerOperationsOrders(row repository.ResellerOperationsOrdersRow) ResellerOperationsOrdersResponse {
	average := decimal.Zero
	if row.ActiveResellersWithOrders > 0 {
		average = decimal.NewFromInt(row.PaidOrders).Div(decimal.NewFromInt(row.ActiveResellersWithOrders))
	}
	return ResellerOperationsOrdersResponse{
		OrdersTotal:                        row.OrdersTotal,
		PaidOrders:                         row.PaidOrders,
		CompletedOrders:                    row.CompletedOrders,
		RefundedOrders:                     row.RefundedOrders,
		SelfDealingBlockedOrders:           row.SelfDealingBlockedOrders,
		ActiveResellersWithOrders:          row.ActiveResellersWithOrders,
		AveragePaidOrdersPerActiveReseller: average.StringFixed(2),
	}
}

func mapResellerOperationsTopResellers(rows []repository.ResellerOperationsTopResellerRow) []ResellerOperationsTopResellerResponse {
	out := make([]ResellerOperationsTopResellerResponse, 0, len(rows))
	for _, row := range rows {
		item := ResellerOperationsTopResellerResponse{
			ResellerID:     row.ResellerID,
			UserID:         row.UserID,
			Email:          strings.TrimSpace(row.Email),
			DisplayName:    strings.TrimSpace(row.DisplayName),
			OrdersTotal:    row.OrdersTotal,
			PaidOrders:     row.PaidOrders,
			ActiveDomains:  row.ActiveDomains,
			SiteConfigured: row.SiteConfigured,
		}
		if row.LastOrderAt != nil {
			item.LastOrderAt = row.LastOrderAt.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out
}

func buildResellerOperationsAlerts(row repository.ResellerOperationsOverviewRow) []ResellerOperationsAlertItem {
	alerts := make([]ResellerOperationsAlertItem, 0, 4)
	add := func(typ, level string, value int64) {
		if value > 0 {
			alerts = append(alerts, ResellerOperationsAlertItem{Type: typ, Level: level, Value: value})
		}
	}
	add("profiles_pending_review", "warning", row.Lifecycle.ProfilesPendingReview)
	add("domains_pending_review", "warning", row.Lifecycle.DomainsPendingReview)
	add("active_profiles_without_site_config", "info", row.Lifecycle.ActiveProfilesWithoutSiteConfig)
	add("self_dealing_blocked_orders", "warning", row.Orders.SelfDealingBlockedOrders)
	return alerts
}

func formatResellerOperationsDecimal(value decimal.Decimal) string {
	return value.Round(2).StringFixed(2)
}

func normalizeResellerOperationsCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}
