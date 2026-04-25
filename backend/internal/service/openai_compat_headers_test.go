package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccount_GetOpenAICompatHeaders(t *testing.T) {
	t.Run("supports object credentials", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"headers": map[string]any{
					" HTTP-Referer ": " https://app.example.com ",
					"X-Title":        "Sub2API",
					"ignored":        123,
					"":               "skip",
				},
			},
		}

		require.Equal(t, map[string]string{
			"HTTP-Referer": "https://app.example.com",
			"X-Title":      "Sub2API",
		}, account.GetOpenAICompatHeaders())
	})

	t.Run("supports json string credentials", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"headers": `{"X-Test":"demo","Empty":"   "}`,
			},
		}

		require.Equal(t, map[string]string{
			"X-Test": "demo",
		}, account.GetOpenAICompatHeaders())
	})

	t.Run("non apikey openai account returns nil", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"headers": map[string]any{"X-Test": "demo"},
			},
		}

		require.Nil(t, account.GetOpenAICompatHeaders())
	})
}

func TestOpenAIGatewayService_BuildUpstreamRequestAppliesOpenAICompatHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"headers": map[string]any{
				"HTTP-Referer": "https://app.example.com",
				"OpenAI-Beta":  "responses=compat",
				"Host":         "blocked.example.com",
			},
		},
	}

	req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", false)
	require.NoError(t, err)
	require.Equal(t, "Bearer token", req.Header.Get("Authorization"))
	require.Equal(t, "https://app.example.com", req.Header.Get("HTTP-Referer"))
	require.Equal(t, "responses=compat", req.Header.Get("OpenAI-Beta"))
	require.Empty(t, req.Header.Get("Host"))
}

func TestOpenAIGatewayService_BuildUpstreamRequestOpenAIPassthroughAppliesOpenAICompatHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))

	svc := &OpenAIGatewayService{cfg: &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"headers": map[string]any{
				"X-Title":  "Sub2API Compat",
				"Upgrade":  "websocket",
				"api-key":  "provider-key",
				"X-Test-2": "demo",
			},
		},
	}

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, []byte(`{"model":"gpt-5"}`), "token")
	require.NoError(t, err)
	require.Equal(t, "Bearer token", req.Header.Get("Authorization"))
	require.Equal(t, "provider-key", req.Header.Get("api-key"))
	require.Equal(t, "Sub2API Compat", req.Header.Get("X-Title"))
	require.Equal(t, "demo", req.Header.Get("X-Test-2"))
	require.Empty(t, req.Header.Get("Upgrade"))
}

func TestOpenAIGatewayService_BuildOpenAIWSHeadersAppliesOpenAICompatHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"headers": map[string]any{
				"X-Title":               "WS Compat",
				"OpenAI-Beta":           "responses=ws-compat",
				"Sec-WebSocket-Protocol": "blocked",
			},
		},
	}

	headers, _ := svc.buildOpenAIWSHeaders(
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		false,
		"",
		"",
		"",
	)

	require.Equal(t, "Bearer token", headers.Get("Authorization"))
	require.Equal(t, "WS Compat", headers.Get("X-Title"))
	require.Equal(t, "responses=ws-compat", headers.Get("OpenAI-Beta"))
	require.Empty(t, headers.Get("Sec-WebSocket-Protocol"))
}
