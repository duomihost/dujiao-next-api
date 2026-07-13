package dto

import (
	"time"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"
)

type ResellerOrderResp struct {
	OrderNo      string       `json:"order_no"`
	Status       string       `json:"status"`
	Currency     string       `json:"currency"`
	TotalAmount  models.Money `json:"total_amount"`
	BaseAmount   models.Money `json:"base_amount"`
	ProfitAmount models.Money `json:"profit_amount"`
	ProfitStatus string       `json:"profit_status"`
	Domain       string       `json:"domain"`
	BuyerLabel   string       `json:"buyer_label"`
	ItemsCount   int          `json:"items_count"`
	CreatedAt    time.Time    `json:"created_at"`
	PaidAt       *time.Time   `json:"paid_at,omitempty"`
}

type ResellerOrderItemResp struct {
	Title               models.JSON  `json:"title"`
	SKUSnapshot         models.JSON  `json:"sku_snapshot"`
	Quantity            int          `json:"quantity"`
	UnitPrice           models.Money `json:"unit_price"`
	TotalPrice          models.Money `json:"total_price"`
	BaseUnitAmount      string       `json:"base_unit_amount,omitempty"`
	ResellerUnitAmount  string       `json:"reseller_unit_amount,omitempty"`
	BaseTotalAmount     string       `json:"base_total_amount,omitempty"`
	ResellerTotalAmount string       `json:"reseller_total_amount,omitempty"`
	ProfitAmount        string       `json:"profit_amount,omitempty"`
}

type ResellerOrderDetailResp struct {
	ResellerOrderResp
	Items []ResellerOrderItemResp `json:"items"`
}

type ResellerOrderStatsResp struct {
	Total      int64            `json:"total"`
	ByStatus   map[string]int64 `json:"by_status"`
	ByCurrency map[string]int64 `json:"by_currency"`
}

func NewResellerOrderResp(row service.ResellerOrderListItem) ResellerOrderResp {
	return ResellerOrderResp{
		OrderNo:      row.OrderNo,
		Status:       row.Status,
		Currency:     row.Currency,
		TotalAmount:  row.TotalAmount,
		BaseAmount:   row.BaseAmount,
		ProfitAmount: row.ProfitAmount,
		ProfitStatus: row.ProfitStatus,
		Domain:       row.Domain,
		BuyerLabel:   row.BuyerLabel,
		ItemsCount:   row.ItemsCount,
		CreatedAt:    row.CreatedAt,
		PaidAt:       row.PaidAt,
	}
}

func NewResellerOrderRespList(rows []service.ResellerOrderListItem) []ResellerOrderResp {
	out := make([]ResellerOrderResp, 0, len(rows))
	for i := range rows {
		out = append(out, NewResellerOrderResp(rows[i]))
	}
	return out
}

func NewResellerOrderDetailResp(row *service.ResellerOrderDetail) ResellerOrderDetailResp {
	if row == nil {
		return ResellerOrderDetailResp{}
	}
	resp := ResellerOrderDetailResp{ResellerOrderResp: NewResellerOrderResp(row.ResellerOrderListItem)}
	for i := range row.Items {
		item := row.Items[i]
		resp.Items = append(resp.Items, ResellerOrderItemResp{
			Title:               item.Title,
			SKUSnapshot:         item.SKUSnapshot,
			Quantity:            item.Quantity,
			UnitPrice:           item.UnitPrice,
			TotalPrice:          item.TotalPrice,
			BaseUnitAmount:      item.BaseUnitAmount,
			ResellerUnitAmount:  item.ResellerUnitAmount,
			BaseTotalAmount:     item.BaseTotalAmount,
			ResellerTotalAmount: item.ResellerTotalAmount,
			ProfitAmount:        item.ProfitAmount,
		})
	}
	return resp
}

func NewResellerOrderStatsResp(stats service.ResellerOrderStats) ResellerOrderStatsResp {
	return ResellerOrderStatsResp{
		Total:      stats.Total,
		ByStatus:   stats.ByStatus,
		ByCurrency: stats.ByCurrency,
	}
}
