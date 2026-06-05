package management

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestAuthenticateManagementKey_LocalhostIPBan_BlocksCorrectKeyDuringBan(t *testing.T) {
	h := &Handler{
		cfg:            &config.Config{},
		failedAttempts: make(map[string]*attemptInfo),
		envSecret:      "test-secret",
	}

	for i := 0; i < 5; i++ {
		allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "wrong-secret")
		if allowed {
			t.Fatalf("expected auth to be denied at attempt %d", i+1)
		}
		if statusCode != http.StatusUnauthorized || errMsg != "invalid management key" {
			t.Fatalf("unexpected auth failure at attempt %d: status=%d msg=%q", i+1, statusCode, errMsg)
		}
	}

	allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "test-secret")
	if allowed {
		t.Fatalf("expected correct key to be denied while banned")
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("expected forbidden status while banned, got %d", statusCode)
	}
	if !strings.HasPrefix(errMsg, "IP banned due to too many failed attempts. Try again in") {
		t.Fatalf("unexpected banned message: %q", errMsg)
	}
}

func TestAuthenticateManagementKey_MissingKeyDoesNotTriggerIPBan(t *testing.T) {
	h := &Handler{
		cfg:            &config.Config{},
		failedAttempts: make(map[string]*attemptInfo),
		envSecret:      "test-secret",
	}

	for i := 0; i < 10; i++ {
		allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "")
		if allowed {
			t.Fatalf("expected missing-key auth to be denied at attempt %d", i+1)
		}
		if statusCode != http.StatusUnauthorized || errMsg != "missing management key" {
			t.Fatalf("unexpected missing-key response at attempt %d: status=%d msg=%q", i+1, statusCode, errMsg)
		}
	}

	allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "test-secret")
	if !allowed {
		t.Fatalf("expected correct key to work after missing-key requests, status=%d msg=%q", statusCode, errMsg)
	}
}

func TestManagementMiddleware_SetsAndAcceptsHttpOnlySessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{
		cfg:            &config.Config{RemoteManagement: config.RemoteManagement{AllowRemote: true}},
		failedAttempts: make(map[string]*attemptInfo),
		envSecret:      "test-secret",
	}

	router := gin.New()
	router.GET("/v0/management/ping", h.Middleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/v0/management/ping", nil)
	firstReq.Header.Set("Authorization", "Bearer test-secret")
	firstRR := httptest.NewRecorder()
	router.ServeHTTP(firstRR, firstReq)
	if firstRR.Code != http.StatusOK {
		t.Fatalf("first request status=%d body=%s", firstRR.Code, firstRR.Body.String())
	}

	var sessionCookie *http.Cookie
	for _, cookie := range firstRR.Result().Cookies() {
		if cookie.Name == managementSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected management session cookie")
	}
	if !sessionCookie.HttpOnly {
		t.Fatalf("expected management session cookie to be HttpOnly")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/v0/management/ping", nil)
	secondReq.AddCookie(sessionCookie)
	secondRR := httptest.NewRecorder()
	router.ServeHTTP(secondRR, secondReq)
	if secondRR.Code != http.StatusOK {
		t.Fatalf("second request with session cookie status=%d body=%s", secondRR.Code, secondRR.Body.String())
	}
}
