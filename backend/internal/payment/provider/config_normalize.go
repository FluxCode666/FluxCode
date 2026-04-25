package provider

import "github.com/Wei-Shaw/sub2api/internal/payment"

var providerConfigAliases = map[string]map[string][]string{
	payment.TypeEasyPay: {
		"notifyUrl": {"notifyUrl", "notify_url"},
		"returnUrl": {"returnUrl", "return_url"},
	},
	payment.TypeAlipay: {
		"appId":      {"appId", "app_id", "sandbox_app_id"},
		"privateKey": {"privateKey", "appPrivateKey", "app_private_key", "sandbox_app_private_key"},
		"publicKey":  {"publicKey", "alipayPublicKey", "alipay_public_key", "sandbox_alipay_public_key"},
		"gateway":    {"gateway", "sandbox_gateway"},
		"notifyUrl":  {"notifyUrl", "notify_url"},
		"returnUrl":  {"returnUrl", "return_url"},
	},
	payment.TypeWxpay: {
		"appId":       {"appId", "app_id"},
		"mchId":       {"mchId", "mch_id", "merchantId", "merchant_id"},
		"privateKey":  {"privateKey", "merchantPrivateKey", "merchant_private_key"},
		"apiV3Key":    {"apiV3Key", "api_v3_key"},
		"publicKey":   {"publicKey", "platformPublicKey", "platform_public_key"},
		"publicKeyId": {"publicKeyId", "public_key_id"},
		"certSerial":  {"certSerial", "cert_serial"},
		"notifyUrl":   {"notifyUrl", "notify_url"},
	},
	payment.TypeStripe: {
		"secretKey":      {"secretKey", "secret_key"},
		"publishableKey": {"publishableKey", "publishable_key"},
		"webhookSecret":  {"webhookSecret", "webhook_secret"},
	},
}

// NormalizeConfig rewrites provider-specific alias keys into the canonical field names
// expected by the frontend and provider implementations.
func NormalizeConfig(providerKey string, config map[string]string) map[string]string {
	if config == nil {
		return nil
	}

	aliases, ok := providerConfigAliases[providerKey]
	if !ok {
		cloned := make(map[string]string, len(config))
		for k, v := range config {
			cloned[k] = v
		}
		return cloned
	}

	aliasToCanonical := make(map[string]string)
	for canonical, keys := range aliases {
		for _, key := range keys {
			aliasToCanonical[key] = canonical
		}
	}

	normalized := make(map[string]string, len(config))
	for k, v := range config {
		if canonical, isAlias := aliasToCanonical[k]; isAlias && canonical != k {
			continue
		}
		normalized[k] = v
	}

	for canonical, keys := range aliases {
		for _, key := range keys {
			if value := config[key]; value != "" {
				normalized[canonical] = value
				break
			}
		}
	}

	return normalized
}
