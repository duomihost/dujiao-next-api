package service

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/dujiao-next/internal/config"
)

type tenantContextKey struct{}

// TenantContext 表示当前请求解析出的主站或分销站上下文。
type TenantContext struct {
	Host              string
	IsMain            bool
	ResellerID        *uint
	ResellerUserID    uint
	PrimaryDomain     string
	Unavailable       bool
	UnavailableReason string
}

func MainTenantContext(host string) TenantContext {
	return TenantContext{Host: NormalizeResellerHost(host), IsMain: true}
}

func ResellerTenantContext(host string, resellerID uint, resellerUserID uint, primaryDomain string) TenantContext {
	id := resellerID
	return TenantContext{
		Host:           NormalizeResellerHost(host),
		IsMain:         false,
		ResellerID:     &id,
		ResellerUserID: resellerUserID,
		PrimaryDomain:  NormalizeResellerHost(primaryDomain),
	}
}

func UnavailableTenantContext(host string, reason string) TenantContext {
	return TenantContext{
		Host:              NormalizeResellerHost(host),
		Unavailable:       true,
		UnavailableReason: strings.TrimSpace(reason),
	}
}

func WithTenantContext(ctx context.Context, tenant TenantContext) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenant)
}

func TenantFromContext(ctx context.Context) (TenantContext, bool) {
	if ctx == nil {
		return TenantContext{}, false
	}
	tenant, ok := ctx.Value(tenantContextKey{}).(TenantContext)
	return tenant, ok
}

func NormalizeResellerHost(raw string) string {
	host := strings.TrimSpace(strings.ToLower(raw))
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "[") {
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = strings.Trim(parsed, "[]")
		}
	} else if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(host, ".")
	host = strings.Trim(host, "[]")
	return host
}

func ResolveResellerRequestHost(req *http.Request, cfg config.ResellerConfig) string {
	if req == nil {
		return ""
	}
	raw := req.Host
	if cfg.TrustedForwardedHost {
		if forwarded := strings.TrimSpace(req.Header.Get("X-Forwarded-Host")); forwarded != "" {
			raw = strings.Split(forwarded, ",")[0]
		}
	}
	return NormalizeResellerHost(raw)
}
