package dujiaopay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/payment/common"
)

const (
	createOrderPath       = "/v1/orders"
	webhookTimeTolerance  = 5 * time.Minute
	defaultNonceByteCount = 16
)

var (
	ErrConfigInvalid    = errors.New("dujiaopay config invalid")
	ErrRequestFailed    = errors.New("dujiaopay request failed")
	ErrResponseInvalid  = errors.New("dujiaopay response invalid")
	ErrSignatureInvalid = errors.New("dujiaopay signature invalid")
	ErrUnsupportedToken = errors.New("dujiaopay token unsupported")
)

// Config 是 DujiaoPay 支付通道配置。
type Config struct {
	APIBaseURL    string `json:"api_base_url"`
	APIKeyID      string `json:"api_key_id"`
	APISecret     string `json:"api_secret"`
	WebhookSecret string `json:"webhook_secret"`
	Chain         string `json:"chain"`
	TokenID       string `json:"token_id"`
	FiatCurrency  string `json:"fiat_currency"`
	SuccessURL    string `json:"success_url"`
	CancelURL     string `json:"cancel_url"`
}

// CreateInput 创建 DujiaoPay 订单输入。
type CreateInput struct {
	MerchantOrderID string
	FiatAmount      string
	SuccessURL      string
	CancelURL       string
	Metadata        map[string]interface{}
}

// CreateResult 是 DujiaoPay 创建订单响应。
type CreateResult struct {
	OrderID       string                 `json:"order_id"`
	Chain         string                 `json:"chain"`
	TokenID       string                 `json:"token_id"`
	PayAddress    string                 `json:"pay_address"`
	PayableAmount string                 `json:"payable_amount"`
	Status        string                 `json:"status"`
	ExpiresAt     string                 `json:"expires_at"`
	CheckoutToken string                 `json:"checkout_token"`
	CheckoutURL   string                 `json:"checkout_url"`
	Raw           map[string]interface{} `json:"-"`
}

// WebhookEvent 是通过签名校验后的 DujiaoPay Webhook 事件。
type WebhookEvent struct {
	EventID         string
	EventType       string
	EventVersion    string
	CreatedAt       *time.Time
	OrderID         string
	MerchantOrderID string
	TxHash          string
	Status          string
	PaidAt          *time.Time
	Raw             map[string]interface{}
}

type createOptions struct {
	now   func() time.Time
	nonce func() string
}

// Option 覆盖创建订单时钟/nonce，主要用于测试。
type Option func(*createOptions)

func WithNowFunc(fn func() time.Time) Option {
	return func(opts *createOptions) {
		if fn != nil {
			opts.now = fn
		}
	}
}

func WithNonceFunc(fn func() string) Option {
	return func(opts *createOptions) {
		if fn != nil {
			opts.nonce = fn
		}
	}
}

func defaultCreateOptions() createOptions {
	return createOptions{
		now:   time.Now,
		nonce: generateNonce,
	}
}

// ParseConfig 把 channel.ConfigJSON 转为 Config。
func ParseConfig(raw map[string]interface{}) (*Config, error) {
	return common.ParseConfig[Config](raw, ErrConfigInvalid)
}

// Normalize 统一配置字段格式。
func (c *Config) Normalize() {
	if c == nil {
		return
	}
	c.APIBaseURL = strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/")
	c.APIKeyID = strings.TrimSpace(c.APIKeyID)
	c.APISecret = strings.TrimSpace(c.APISecret)
	c.WebhookSecret = strings.TrimSpace(c.WebhookSecret)
	c.Chain = strings.ToLower(strings.TrimSpace(c.Chain))
	c.TokenID = strings.ToLower(strings.TrimSpace(c.TokenID))
	c.FiatCurrency = strings.ToUpper(strings.TrimSpace(c.FiatCurrency))
	c.SuccessURL = strings.TrimSpace(c.SuccessURL)
	c.CancelURL = strings.TrimSpace(c.CancelURL)
	if c.FiatCurrency == "" {
		c.FiatCurrency = strings.ToUpper(constants.SiteCurrencyDefault)
	}
	if c.Chain == "" && c.TokenID != "" {
		c.Chain = ResolveChain(c.TokenID)
	}
}

// ValidateConfig 校验 DujiaoPay 必填配置。
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	cfg.Normalize()
	checks := []struct {
		field string
		value string
	}{
		{"api_base_url", cfg.APIBaseURL},
		{"api_key_id", cfg.APIKeyID},
		{"api_secret", cfg.APISecret},
		{"webhook_secret", cfg.WebhookSecret},
		{"token_id", cfg.TokenID},
		{"fiat_currency", cfg.FiatCurrency},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.value) == "" {
			return fmt.Errorf("%w: %s is required", ErrConfigInvalid, check.field)
		}
	}
	if !IsSupportedTokenID(cfg.TokenID) {
		return fmt.Errorf("%w: %s", ErrUnsupportedToken, cfg.TokenID)
	}
	if cfg.Chain == "" {
		return fmt.Errorf("%w: chain is required", ErrConfigInvalid)
	}
	return nil
}

// ResolveChain 根据 DujiaoPay token_id 推导 chain。
func ResolveChain(tokenID string) string {
	tokenID = strings.ToLower(strings.TrimSpace(tokenID))
	if chain, ok := supportedTokenChains[tokenID]; ok {
		return chain
	}
	return ""
}

// IsSupportedTokenID 判断 token_id 是否在 DujiaoPay 文档列出的支持范围内。
func IsSupportedTokenID(tokenID string) bool {
	_, ok := supportedTokenChains[strings.ToLower(strings.TrimSpace(tokenID))]
	return ok
}

var supportedTokenChains = map[string]string{
	"tron-trx":       "tron",
	"tron-usdt":      "tron",
	"ethereum-eth":   "ethereum",
	"ethereum-usdt":  "ethereum",
	"ethereum-usdc":  "ethereum",
	"bsc-bnb":        "bsc",
	"bsc-usdt":       "bsc",
	"polygon-usdc":   "polygon",
	"polygon-usdt0":  "polygon",
	"base-usdc":      "base",
	"arbitrum-usdc":  "arbitrum",
	"arbitrum-usdt0": "arbitrum",
	"plasma-usdt0":   "plasma",
	"x-layer-usdt0":  "x-layer",
	"solana-usdc":    "solana",
	"solana-usdt":    "solana",
	"aptos-usdc":     "aptos",
	"aptos-usdt":     "aptos",
}

// SignHeaders 按 DujiaoPay 文档构造 API 请求 HMAC 头。
func SignHeaders(secret, keyID, method, path, rawQuery string, body []byte, unixTimestamp int64, nonce string) map[string]string {
	sum := sha256.Sum256(body)
	timestamp := strconv.FormatInt(unixTimestamp, 10)
	canonical := strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(method)),
		normalizeCanonicalPath(path),
		canonicalQuery(rawQuery),
		hex.EncodeToString(sum[:]),
		timestamp,
		strings.TrimSpace(nonce),
	}, "\n")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))

	return map[string]string{
		"DJP-Key-ID":    strings.TrimSpace(keyID),
		"DJP-Timestamp": timestamp,
		"DJP-Nonce":     strings.TrimSpace(nonce),
		"DJP-Signature": hex.EncodeToString(mac.Sum(nil)),
	}
}

// CreatePayment 创建 DujiaoPay 订单。
func CreatePayment(ctx context.Context, cfg *Config, input CreateInput, options ...Option) (*CreateResult, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.MerchantOrderID) == "" || strings.TrimSpace(input.FiatAmount) == "" {
		return nil, fmt.Errorf("%w: merchant_order_id and fiat_amount are required", ErrConfigInvalid)
	}

	opts := defaultCreateOptions()
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}

	successURL := strings.TrimSpace(input.SuccessURL)
	if successURL == "" {
		successURL = cfg.SuccessURL
	}
	cancelURL := strings.TrimSpace(input.CancelURL)
	if cancelURL == "" {
		cancelURL = cfg.CancelURL
	}

	payload := map[string]interface{}{
		"chain":             cfg.Chain,
		"token_id":          cfg.TokenID,
		"fiat_currency":     cfg.FiatCurrency,
		"fiat_amount":       strings.TrimSpace(input.FiatAmount),
		"merchant_order_id": strings.TrimSpace(input.MerchantOrderID),
	}
	if successURL != "" {
		payload["success_url"] = successURL
	}
	if cancelURL != "" {
		payload["cancel_url"] = cancelURL
	}
	if len(input.Metadata) > 0 {
		payload["metadata"] = input.Metadata
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal create order payload failed", ErrConfigInvalid)
	}

	ctx, cancel := common.WithDefaultTimeout(ctx)
	defer cancel()

	endpoint := cfg.APIBaseURL + createOrderPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request failed", ErrRequestFailed)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", strings.TrimSpace(input.MerchantOrderID))
	for key, value := range SignHeaders(cfg.APISecret, cfg.APIKeyID, http.MethodPost, createOrderPath, "", body, opts.now().Unix(), opts.nonce()) {
		req.Header.Set(key, value)
	}

	resp, err := (&http.Client{Timeout: common.DefaultTimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response failed", ErrRequestFailed)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: status=%d body=%s", ErrRequestFailed, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode response failed", ErrResponseInvalid)
	}
	result := parseCreateResult(raw)
	if result.OrderID == "" || result.CheckoutURL == "" {
		return nil, fmt.Errorf("%w: missing order_id/checkout_url", ErrResponseInvalid)
	}
	result.Raw = raw
	return result, nil
}

// ParseWebhook 校验 DujiaoPay webhook 签名并解析事件。
func ParseWebhook(cfg *Config, headers map[string]string, body []byte, now time.Time) (*WebhookEvent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	cfg.Normalize()
	if cfg.WebhookSecret == "" {
		return nil, fmt.Errorf("%w: webhook_secret is required", ErrConfigInvalid)
	}
	timestamp := headerValue(headers, "DJP-Webhook-Timestamp")
	signature := headerValue(headers, "DJP-Webhook-Signature")
	if timestamp == "" || signature == "" {
		return nil, fmt.Errorf("%w: missing webhook signature headers", ErrSignatureInvalid)
	}
	if now.IsZero() {
		now = time.Now()
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid webhook timestamp", ErrSignatureInvalid)
	}
	eventTime := time.Unix(ts, 0)
	if now.Sub(eventTime) > webhookTimeTolerance || eventTime.Sub(now) > webhookTimeTolerance {
		return nil, fmt.Errorf("%w: webhook timestamp outside tolerance", ErrSignatureInvalid)
	}

	mac := hmac.New(sha256.New, []byte(cfg.WebhookSecret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	want := mac.Sum(nil)
	got, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(signature), "sha256="))
	if err != nil || !hmac.Equal(got, want) {
		return nil, fmt.Errorf("%w: webhook signature mismatch", ErrSignatureInvalid)
	}

	var envelope webhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%w: decode webhook failed", ErrResponseInvalid)
	}
	var raw map[string]interface{}
	_ = json.Unmarshal(body, &raw)

	createdAt := parseTimePtr(envelope.CreatedAt)
	paidAt := parseTimePtr(envelope.Data.PaidAt)
	if paidAt == nil && envelope.EventType == "order.paid" {
		paidAt = createdAt
	}

	eventID := strings.TrimSpace(envelope.EventID)
	if eventID == "" {
		eventID = strings.TrimSpace(headerValue(headers, "DJP-Webhook-ID"))
	}

	return &WebhookEvent{
		EventID:         eventID,
		EventType:       strings.TrimSpace(envelope.EventType),
		EventVersion:    strings.TrimSpace(envelope.EventVersion),
		CreatedAt:       createdAt,
		OrderID:         strings.TrimSpace(envelope.Data.OrderID),
		MerchantOrderID: strings.TrimSpace(envelope.Data.MerchantOrderID),
		TxHash:          strings.TrimSpace(envelope.Data.TxHash),
		Status:          toPaymentStatus(envelope.EventType),
		PaidAt:          paidAt,
		Raw:             raw,
	}, nil
}

type webhookEnvelope struct {
	EventID      string      `json:"event_id"`
	EventType    string      `json:"event_type"`
	EventVersion string      `json:"event_version"`
	CreatedAt    string      `json:"created_at"`
	Data         webhookData `json:"data"`
}

type webhookData struct {
	OrderID         string `json:"order_id"`
	MerchantOrderID string `json:"merchant_order_id"`
	TxHash          string `json:"tx_hash"`
	PaidAt          string `json:"paid_at"`
}

func parseCreateResult(raw map[string]interface{}) *CreateResult {
	source := raw
	if data := common.ReadMap(raw, "data"); data != nil {
		source = data
	}
	return &CreateResult{
		OrderID:       common.ReadString(source, "order_id"),
		Chain:         common.ReadString(source, "chain"),
		TokenID:       common.ReadString(source, "token_id"),
		PayAddress:    common.ReadString(source, "pay_address"),
		PayableAmount: common.ReadString(source, "payable_amount"),
		Status:        common.ReadString(source, "status"),
		ExpiresAt:     common.ReadString(source, "expires_at"),
		CheckoutToken: common.ReadString(source, "checkout_token"),
		CheckoutURL:   common.ReadString(source, "checkout_url"),
	}
}

func toPaymentStatus(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case "order.paid":
		return constants.PaymentStatusSuccess
	case "order.expired":
		return constants.PaymentStatusExpired
	case "order.canceled":
		return constants.PaymentStatusFailed
	default:
		return ""
	}
}

func generateNonce() string {
	buf := make([]byte, defaultNonceByteCount)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf)
}

func normalizeCanonicalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func canonicalQuery(rawQuery string) string {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	return values.Encode()
}

func headerValue(headers map[string]string, name string) string {
	if len(headers) == 0 {
		return ""
	}
	if value := strings.TrimSpace(headers[name]); value != "" {
		return value
	}
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseTimePtr(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}
