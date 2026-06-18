package provider

import (
	"net/url"
	"strings"
)

// appendQueryParams 向 URL 追加查询参数,空 url 或参数为空时跳过。
// 行为等价于 service 层 appendURLQuery,放 provider 包内供 adapter wrapper 共用,
// 避免反向依赖 service 包。
func appendQueryParams(rawURL string, params map[string]string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if len(params) == 0 {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	for key, value := range params {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
