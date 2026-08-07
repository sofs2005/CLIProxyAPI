package xai

import (
	"encoding/base64"
	"testing"
)

func TestIsBFSAccessToken(t *testing.T) {
	tests := []struct {
		name  string
		claim string
		want  bool
	}{
		{name: "numeric one", claim: `{"bfs":1}`, want: true},
		{name: "numeric one point zero", claim: `{"bfs":1.0}`, want: true},
		{name: "numeric zero", claim: `{"bfs":0}`},
		{name: "string one", claim: `{"bfs":"1"}`},
		{name: "boolean true", claim: `{"bfs":true}`},
		{name: "missing", claim: `{"sub":"xai-user"}`},
		{name: "invalid jwt", claim: "not-a-jwt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := tt.claim
			if tt.name != "invalid jwt" {
				payload := base64.RawURLEncoding.EncodeToString([]byte(tt.claim))
				token = "eyJhbGciOiJub25lIn0." + payload + ".signature"
			}
			if got := IsBFSAccessToken(token); got != tt.want {
				t.Fatalf("IsBFSAccessToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeJWTPayloadAcceptsPaddedPayload(t *testing.T) {
	payload := base64.URLEncoding.EncodeToString([]byte(`{"bfs":1}`))
	claims, ok := decodeJWTPayload("header." + payload + ".signature")
	if !ok || claims["bfs"] != float64(1) {
		t.Fatalf("decodeJWTPayload() = %#v, %v", claims, ok)
	}
}
