package provider

import (
	"strings"
	"sync"
)

// Registry 按 (providerType, channelType) 路由 Provider 实例。
//
// 注册策略:
//   - PaymentProviderOfficial 类型按 channelType 细分(official:paypal / official:stripe)
//   - 其它单一 channelType 的 provider(epay/epusdt 等)注册时 channelType 传 "",
//     Lookup 会先精确匹配,失败 fallback 到 (providerType, "")。
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register 注册 provider。重复 Register 同一 key 会覆盖。
func (r *Registry) Register(providerType, channelType string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[keyFor(providerType, channelType)] = p
}

// Lookup 按 (providerType, channelType) 精确匹配,失败 fallback 到
// (providerType, "")。两者都没有则返回 (nil, false)。
func (r *Registry) Lookup(providerType, channelType string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.providers[keyFor(providerType, channelType)]; ok {
		return p, true
	}
	if p, ok := r.providers[keyFor(providerType, "")]; ok {
		return p, true
	}
	return nil, false
}

func keyFor(providerType, channelType string) string {
	p := strings.ToLower(strings.TrimSpace(providerType))
	c := strings.ToLower(strings.TrimSpace(channelType))
	return p + ":" + c
}
