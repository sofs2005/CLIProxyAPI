package xai

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// IsBFSAccessToken reports whether an xAI access token contains the numeric bfs=1 marker.
func IsBFSAccessToken(token string) bool {
	claims, ok := decodeJWTPayload(token)
	if !ok {
		return false
	}
	value, ok := claims["bfs"]
	if !ok {
		return false
	}
	switch number := value.(type) {
	case float64:
		return number == 1
	case float32:
		return number == 1
	case int:
		return number == 1
	case int8:
		return number == 1
	case int16:
		return number == 1
	case int32:
		return number == 1
	case int64:
		return number == 1
	case uint:
		return number == 1
	case uint8:
		return number == 1
	case uint16:
		return number == 1
	case uint32:
		return number == 1
	case uint64:
		return number == 1
	case json.Number:
		return number == "1" || number == "1.0"
	default:
		return false
	}
}

func decodeJWTPayload(token string) (map[string]any, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return nil, false
	}
	payload := parts[1]
	raw, errDecode := base64.RawURLEncoding.DecodeString(payload)
	if errDecode != nil {
		padded := payload + strings.Repeat("=", (4-len(payload)%4)%4)
		raw, errDecode = base64.URLEncoding.DecodeString(padded)
		if errDecode != nil {
			return nil, false
		}
	}
	var claims map[string]any
	if errUnmarshal := json.Unmarshal(raw, &claims); errUnmarshal != nil {
		return nil, false
	}
	return claims, true
}
