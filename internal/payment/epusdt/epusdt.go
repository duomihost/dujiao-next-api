package epusdt

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/payment/common"
)

var (
	ErrConfigInvalid    = errors.New("epusdt config invalid")
	ErrRequestFailed    = errors.New("epusdt request failed")
	ErrResponseInvalid  = errors.New("epusdt response invalid")
	ErrSignatureInvalid = errors.New("epusdt signature invalid")
)

// 订单状态常量（epusdt 文档未明示具体取值，按通用约定实现并对未知值兜底）
const (
	StatusWaiting = 1
	StatusSuccess = 2
	StatusExpired = 3

	gmpayCreateTransactionPath = "/payments/gmpay/v1/order/create-transaction"
	checkoutCounterPathPrefix  = "/pay/checkout-counter/"
)

// Config epusdt（GMPay）配置
type Config struct {
	GatewayURL  string `json:"gateway_url"`
	PID         string `json:"pid"`
	SecretKey   string `json:"secret_key"`
	Token       string `json:"token"`
	Network     string `json:"network"`
	Currency    string `json:"currency,omitempty"`
	NotifyURL   string `json:"notify_url"`
	ReturnURL   string `json:"return_url"`
	PaymentType string `json:"payment_type,omitempty"`
}

// ParseConfig 把 channel.ConfigJSON 反序列化为 Config
func ParseConfig(raw map[string]interface{}) (*Config, error) {
	return common.ParseConfig[Config](raw, ErrConfigInvalid)
}

// Normalize 统一字段格式
func (c *Config) Normalize() {
	if c == nil {
		return
	}
	c.GatewayURL = strings.TrimRight(strings.TrimSpace(c.GatewayURL), "/")
	c.PID = strings.TrimSpace(c.PID)
	c.SecretKey = strings.TrimSpace(c.SecretKey)
	c.Token = strings.ToLower(strings.TrimSpace(c.Token))
	c.Network = strings.ToLower(strings.TrimSpace(c.Network))
	c.Currency = strings.ToLower(strings.TrimSpace(c.Currency))
	c.NotifyURL = strings.TrimSpace(c.NotifyURL)
	c.ReturnURL = strings.TrimSpace(c.ReturnURL)
	c.PaymentType = strings.TrimSpace(c.PaymentType)
	if c.Currency == "" {
		c.Currency = strings.ToLower(constants.SiteCurrencyDefault)
	}
}

// ValidateConfig 校验必填字段
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	checks := []struct {
		field string
		val   string
	}{
		{"gateway_url", cfg.GatewayURL},
		{"pid", cfg.PID},
		{"secret_key", cfg.SecretKey},
		{"token", cfg.Token},
		{"network", cfg.Network},
		{"notify_url", cfg.NotifyURL},
		{"return_url", cfg.ReturnURL},
	}
	for _, c := range checks {
		if strings.TrimSpace(c.val) == "" {
			return fmt.Errorf("%w: %s is required", ErrConfigInvalid, c.field)
		}
	}
	return nil
}

// Sign 计算签名。算法：剔除 signature 与空值，key 升序，按 key=value 用 & 连接，
// 末尾直接拼接 secret_key（无分隔符），MD5 小写 hex。
func Sign(params map[string]interface{}, secretKey string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "signature" {
			continue
		}
		if isEmptyValue(v) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, params[k]))
	}

	content := strings.Join(pairs, "&") + secretKey
	sum := md5.Sum([]byte(content))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

// ToPaymentStatus 把 epusdt 状态码映射为内部 payment 状态
func ToPaymentStatus(status int) string {
	switch status {
	case StatusSuccess:
		return constants.PaymentStatusSuccess
	case StatusExpired:
		return constants.PaymentStatusExpired
	default:
		return constants.PaymentStatusPending
	}
}

// CreateInput 创建订单输入
type CreateInput struct {
	OrderNo   string
	Amount    string
	Name      string
	NotifyURL string
	ReturnURL string
}

// CreateResult 创建订单结果
type CreateResult struct {
	TradeID    string
	PaymentURL string // {GatewayURL}/pay/checkout-counter/{TradeID}
	Raw        map[string]interface{}
}

// CallbackData 回调数据
type CallbackData struct {
	PID                string      `json:"pid"`
	TradeID            string      `json:"trade_id"`
	OrderID            string      `json:"order_id"`
	Amount             interface{} `json:"amount"`
	ActualAmount       interface{} `json:"actual_amount"`
	ReceiveAddress     string      `json:"receive_address"`
	Token              string      `json:"token"`
	BlockTransactionID string      `json:"block_transaction_id"`
	Status             int         `json:"status"`
	Signature          string      `json:"signature"`
}

// GetAmount 兼容 float64 / string 两种 JSON 类型
func (c *CallbackData) GetAmount() float64       { return toFloat(c.Amount) }
func (c *CallbackData) GetActualAmount() float64 { return toFloat(c.ActualAmount) }

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
			return f
		}
	}
	return 0
}

// CreatePayment 调 POST /payments/gmpay/v1/order/create-transaction，返回 trade_id 和拼好的收银台 URL。
func CreatePayment(ctx context.Context, cfg *Config, input CreateInput) (*CreateResult, error) {
	if cfg == nil {
		return nil, ErrConfigInvalid
	}
	if strings.TrimSpace(input.OrderNo) == "" || strings.TrimSpace(input.Amount) == "" {
		return nil, fmt.Errorf("%w: order_no and amount are required", ErrConfigInvalid)
	}
	if len(input.OrderNo) > 32 {
		return nil, fmt.Errorf("%w: order_no exceeds 32 chars", ErrConfigInvalid)
	}

	amount, err := strconv.ParseFloat(input.Amount, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid amount %q", ErrConfigInvalid, input.Amount)
	}
	if amount <= 0.01 {
		return nil, fmt.Errorf("%w: amount must be greater than 0.01", ErrConfigInvalid)
	}

	notifyURL := input.NotifyURL
	if strings.TrimSpace(notifyURL) == "" {
		notifyURL = cfg.NotifyURL
	}
	returnURL := input.ReturnURL
	if strings.TrimSpace(returnURL) == "" {
		returnURL = cfg.ReturnURL
	}

	params := map[string]interface{}{
		"pid":          cfg.PID,
		"order_id":     input.OrderNo,
		"currency":     cfg.Currency,
		"token":        cfg.Token,
		"network":      cfg.Network,
		"amount":       amount,
		"notify_url":   notifyURL,
		"redirect_url": returnURL,
	}
	if strings.TrimSpace(input.Name) != "" {
		params["name"] = input.Name
	}
	if strings.TrimSpace(cfg.PaymentType) != "" {
		params["payment_type"] = cfg.PaymentType
	}
	params["signature"] = Sign(params, cfg.SecretKey)

	endpoint := cfg.GatewayURL + gmpayCreateTransactionPath
	respBody, err := postJSON(ctx, endpoint, params)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResponseInvalid, err)
	}

	tradeID := extractTradeID(raw)
	if tradeID == "" {
		return nil, fmt.Errorf("%w: trade_id missing in response", ErrResponseInvalid)
	}

	return &CreateResult{
		TradeID:    tradeID,
		PaymentURL: cfg.GatewayURL + checkoutCounterPathPrefix + tradeID,
		Raw:        raw,
	}, nil
}

// extractTradeID 宽松解析：顶层 / data.trade_id / data.id 任一命中
func extractTradeID(raw map[string]interface{}) string {
	if v, ok := raw["trade_id"].(string); ok && v != "" {
		return v
	}
	if data, ok := raw["data"].(map[string]interface{}); ok {
		if v, ok := data["trade_id"].(string); ok && v != "" {
			return v
		}
		if v, ok := data["id"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func postJSON(ctx context.Context, endpoint string, params map[string]interface{}) ([]byte, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ParseCallback 解析 epusdt 回调 JSON
func ParseCallback(body []byte) (*CallbackData, error) {
	if len(body) == 0 {
		return nil, ErrResponseInvalid
	}
	var data CallbackData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResponseInvalid, err)
	}
	return &data, nil
}

// VerifyCallback 验签 + 状态校验。仅 status==StatusSuccess 视为合法成功通知。
func VerifyCallback(cfg *Config, data *CallbackData) error {
	if cfg == nil || data == nil {
		return ErrConfigInvalid
	}
	if data.Status != StatusSuccess {
		return fmt.Errorf("%w: status=%d", ErrResponseInvalid, data.Status)
	}

	params := map[string]interface{}{
		"pid":                  data.PID,
		"trade_id":             data.TradeID,
		"order_id":             data.OrderID,
		"amount":               data.GetAmount(),
		"actual_amount":        data.GetActualAmount(),
		"receive_address":      data.ReceiveAddress,
		"token":                data.Token,
		"block_transaction_id": data.BlockTransactionID,
		"status":               data.Status,
	}
	expected := Sign(params, cfg.SecretKey)
	if !strings.EqualFold(expected, data.Signature) {
		return ErrSignatureInvalid
	}
	return nil
}
