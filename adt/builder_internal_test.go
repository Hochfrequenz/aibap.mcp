package adt

import (
	"testing"

	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

func TestEffectiveOAuth2ClientID(t *testing.T) {
	tests := []struct {
		name     string
		sys      sapmcpconfig.SAPSystem
		fallback string
		want     string
	}{
		{
			name:     "system clientID wins over fallback",
			sys:      sapmcpconfig.SAPSystem{OAuth2ClientID: "system-id"},
			fallback: "fallback-id",
			want:     "system-id",
		},
		{
			name:     "fallback used when system clientID empty",
			sys:      sapmcpconfig.SAPSystem{},
			fallback: "fallback-id",
			want:     "fallback-id",
		},
		{
			name:     "both empty returns empty",
			sys:      sapmcpconfig.SAPSystem{},
			fallback: "",
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveOAuth2ClientID(tt.sys, tt.fallback); got != tt.want {
				t.Errorf("effectiveOAuth2ClientID = %q, want %q", got, tt.want)
			}
		})
	}
}
