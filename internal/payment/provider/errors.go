package provider

import "errors"

// 统一的支付 provider 错误。各 adapter wrapper 把 native error
// (例如 stripe.ErrConfigInvalid / paypal.ErrRequestFailed) 映射成这些
// sentinel,调用方一律用 errors.Is 判定。
var (
	ErrConfigInvalid      = errors.New("payment provider config invalid")
	ErrRequestFailed      = errors.New("payment provider request failed")
	ErrResponseInvalid    = errors.New("payment provider response invalid")
	ErrSignatureInvalid   = errors.New("payment provider signature invalid")
	ErrAuthFailed         = errors.New("payment provider auth failed")
	ErrUnsupportedChannel = errors.New("payment channel type not supported by provider")
	ErrProviderNotFound   = errors.New("payment provider not found in registry")
)
