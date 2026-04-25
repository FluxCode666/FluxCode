//go:build unit

package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIsTradeNotExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error returns false",
			err:  nil,
			want: false,
		},
		{
			name: "error containing ACQ.TRADE_NOT_EXIST returns true",
			err:  errors.New("alipay: sub_code=ACQ.TRADE_NOT_EXIST, sub_msg=交易不存在"),
			want: true,
		},
		{
			name: "error not containing the code returns false",
			err:  errors.New("alipay: sub_code=ACQ.SYSTEM_ERROR, sub_msg=系统错误"),
			want: false,
		},
		{
			name: "error with only partial match returns false",
			err:  errors.New("ACQ.TRADE_NOT"),
			want: false,
		},
		{
			name: "error with exact constant value returns true",
			err:  errors.New(alipayErrTradeNotExist),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isTradeNotExist(tt.err)
			if got != tt.want {
				t.Errorf("isTradeNotExist(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestNewAlipay(t *testing.T) {
	t.Parallel()

	privateKey, publicKey := mustGenerateAlipayTestKeys(t)
	validConfig := map[string]string{
		"appId":      "2021001234567890",
		"privateKey": privateKey,
		"publicKey":  publicKey,
	}

	// helper to clone and override config fields
	withOverride := func(overrides map[string]string) map[string]string {
		cfg := make(map[string]string, len(validConfig))
		for k, v := range validConfig {
			cfg[k] = v
		}
		for k, v := range overrides {
			cfg[k] = v
		}
		return cfg
	}

	tests := []struct {
		name      string
		config    map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid config succeeds",
			config:  validConfig,
			wantErr: false,
		},
		{
			name:      "missing appId",
			config:    withOverride(map[string]string{"appId": ""}),
			wantErr:   true,
			errSubstr: "appId",
		},
		{
			name:      "missing privateKey",
			config:    withOverride(map[string]string{"privateKey": ""}),
			wantErr:   true,
			errSubstr: "privateKey",
		},
		{
			name:      "nil config map returns error for appId",
			config:    map[string]string{},
			wantErr:   true,
			errSubstr: "appId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewAlipay("test-instance", tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected non-nil Alipay instance")
			}
			if got.instanceID != "test-instance" {
				t.Errorf("instanceID = %q, want %q", got.instanceID, "test-instance")
			}
		})
	}
}

func TestNewAlipayAcceptsSandboxAliases(t *testing.T) {
	t.Parallel()

	privateKey, publicKey := mustGenerateAlipayTestKeys(t)
	got, err := NewAlipay("test-instance", map[string]string{
		"sandbox_app_id":            "9021000136698392",
		"sandbox_app_private_key":   privateKey,
		"sandbox_alipay_public_key": publicKey,
		"sandbox_gateway":           "https://openapi-sandbox.dl.alipaydev.com/gateway.do",
	})
	if err != nil {
		t.Fatalf("NewAlipay returned error: %v", err)
	}
	if got.config["appId"] != "9021000136698392" {
		t.Fatalf("appId = %q, want sandbox appId", got.config["appId"])
	}
	if got.config["gateway"] != "https://openapi-sandbox.dl.alipaydev.com/gateway.do" {
		t.Fatalf("gateway = %q, want sandbox gateway", got.config["gateway"])
	}
	if got.production {
		t.Fatal("expected sandbox config to mark provider as non-production")
	}
}

func TestAlipayGetClientUsesConfiguredGateway(t *testing.T) {
	t.Parallel()

	privateKey, publicKey := mustGenerateAlipayTestKeys(t)
	got, err := NewAlipay("test-instance", map[string]string{
		"appId":      "9021000136698392",
		"privateKey": privateKey,
		"publicKey":  publicKey,
		"gateway":    "https://openapi-sandbox.dl.alipaydev.com/gateway.do",
	})
	if err != nil {
		t.Fatalf("NewAlipay returned error: %v", err)
	}

	client, err := got.getClient()
	if err != nil {
		t.Fatalf("getClient returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if host := reflect.ValueOf(client).Elem().FieldByName("host").String(); host != "https://openapi-sandbox.dl.alipaydev.com/gateway.do" {
		t.Fatalf("client host = %q, want sandbox gateway", host)
	}
	if client.Production() {
		t.Fatal("expected sandbox gateway to create non-production client")
	}
}

func mustGenerateAlipayTestKeys(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate test private key: %v", err)
	}

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal test private key: %v", err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal test public key: %v", err)
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})

	return string(privateKeyPEM), string(publicKeyPEM)
}
