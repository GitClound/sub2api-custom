package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const cliProxyAuthBundleType = "cliproxy-auth-bundle"

type CLIProxyAuthImportRequest struct {
	Files                []CLIProxyAuthFile `json:"files"`
	SkipDefaultGroupBind *bool              `json:"skip_default_group_bind"`
	Concurrency          int                `json:"concurrency"`
	Priority             int                `json:"priority"`
}

type CLIProxyAuthFile struct {
	Name string         `json:"name"`
	Data map[string]any `json:"data"`
}

type CLIProxyAuthExportPayload struct {
	Type       string             `json:"type"`
	Version    int                `json:"version"`
	ExportedAt string             `json:"exported_at"`
	Files      []CLIProxyAuthFile `json:"files"`
}

func (h *AccountHandler) ImportCLIProxyAuthFiles(c *gin.Context) {
	var req CLIProxyAuthImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	dataReq, err := buildDataImportRequestFromCLIProxyAuths(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_cliproxy_auths", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importData(ctx, dataReq)
	})
}

func (h *AccountHandler) ExportCLIProxyAuthFiles(c *gin.Context) {
	ctx := c.Request.Context()
	selectedIDs, err := parseAccountIDs(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	accounts, err := h.resolveExportAccounts(ctx, selectedIDs, c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	files := make([]CLIProxyAuthFile, 0, len(accounts))
	for i := range accounts {
		file, ok := buildCLIProxyAuthFileFromAccount(accounts[i])
		if ok {
			files = append(files, file)
		}
	}

	response.Success(c, CLIProxyAuthExportPayload{
		Type:       cliProxyAuthBundleType,
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Files:      files,
	})
}

func buildDataImportRequestFromCLIProxyAuths(req CLIProxyAuthImportRequest) (DataImportRequest, error) {
	files := flattenCLIProxyAuthFiles(req.Files)
	if len(files) == 0 {
		return DataImportRequest{}, fmt.Errorf("files is required")
	}

	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 3
	}
	priority := req.Priority
	if priority <= 0 {
		priority = 50
	}

	accounts := make([]DataAccount, 0, len(files))
	var errors []string
	for _, file := range files {
		account, err := buildDataAccountFromCLIProxyAuth(file, concurrency, priority)
		if err != nil {
			name := strings.TrimSpace(file.Name)
			if name == "" {
				name = "auth"
			}
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		accounts = append(accounts, account)
	}
	if len(accounts) == 0 {
		return DataImportRequest{}, fmt.Errorf("no supported CLIProxyAPI auth files found: %s", strings.Join(errors, "; "))
	}

	skipDefaultGroupBind := true
	if req.SkipDefaultGroupBind != nil {
		skipDefaultGroupBind = *req.SkipDefaultGroupBind
	}

	return DataImportRequest{
		Data: DataPayload{
			Type:       dataType,
			Version:    dataVersion,
			ExportedAt: time.Now().UTC().Format(time.RFC3339),
			Proxies:    []DataProxy{},
			Accounts:   accounts,
		},
		SkipDefaultGroupBind: &skipDefaultGroupBind,
	}, nil
}

func flattenCLIProxyAuthFiles(files []CLIProxyAuthFile) []CLIProxyAuthFile {
	out := make([]CLIProxyAuthFile, 0, len(files))
	for _, file := range files {
		if strings.EqualFold(stringValue(file.Data, "type"), cliProxyAuthBundleType) {
			for _, nested := range parseCLIProxyBundleFiles(file.Data) {
				out = append(out, nested)
			}
			continue
		}
		out = append(out, file)
	}
	return out
}

func parseCLIProxyBundleFiles(data map[string]any) []CLIProxyAuthFile {
	rawFiles, ok := data["files"].([]any)
	if !ok {
		return nil
	}
	out := make([]CLIProxyAuthFile, 0, len(rawFiles))
	for _, raw := range rawFiles {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := stringValue(item, "name")
		fileData, _ := item["data"].(map[string]any)
		if len(fileData) == 0 {
			continue
		}
		out = append(out, CLIProxyAuthFile{Name: name, Data: fileData})
	}
	return out
}

func buildDataAccountFromCLIProxyAuth(file CLIProxyAuthFile, concurrency, priority int) (DataAccount, error) {
	data := file.Data
	if len(data) == 0 {
		return DataAccount{}, fmt.Errorf("auth data is empty")
	}

	provider := strings.ToLower(strings.TrimSpace(stringValue(data, "type")))
	credentials := make(map[string]any)
	extra := map[string]any{"cliproxy_auth_file": filepath.Base(file.Name)}
	platform := ""
	name := ""

	switch provider {
	case "codex", "openai":
		platform = service.PlatformOpenAI
		copyStringFields(credentials, data, "access_token", "refresh_token", "id_token", "email", "client_id")
		setIfNotEmpty(credentials, "chatgpt_account_id", firstStringValue(data, "chatgpt_account_id", "account_id"))
		setIfNotEmpty(credentials, "plan_type", firstStringValue(data, "plan_type"))
		setIfNotEmpty(credentials, "expires_at", firstStringValue(data, "expires_at", "expired", "expire"))
		name = firstStringValue(data, "email", "account_id")
	case "gemini":
		platform = service.PlatformGemini
		copyGoogleToken(credentials, data)
		copyStringFields(credentials, data, "email", "project_id", "oauth_type", "tier_id", "scope", "token_type")
		if credentials["oauth_type"] == nil && strings.TrimSpace(stringValue(credentials, "project_id")) != "" {
			credentials["oauth_type"] = "code_assist"
		}
		name = firstStringValue(data, "email", "project_id")
	case "antigravity":
		platform = service.PlatformAntigravity
		copyGoogleToken(credentials, data)
		copyStringFields(credentials, data, "email", "project_id", "plan_type", "token_type")
		name = firstStringValue(data, "email", "project_id")
	default:
		return DataAccount{}, fmt.Errorf("unsupported auth type %q", provider)
	}

	if strings.TrimSpace(stringValue(credentials, "refresh_token")) == "" && strings.TrimSpace(stringValue(credentials, "access_token")) == "" {
		return DataAccount{}, fmt.Errorf("missing access_token or refresh_token")
	}

	if name == "" {
		name = strings.TrimSuffix(filepath.Base(file.Name), filepath.Ext(file.Name))
	}
	if name == "" {
		name = platform + " OAuth Account"
	}

	return DataAccount{
		Name:        name,
		Platform:    platform,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		Extra:       extra,
		Concurrency: concurrency,
		Priority:    priority,
	}, nil
}

func buildCLIProxyAuthFileFromAccount(account service.Account) (CLIProxyAuthFile, bool) {
	if account.Type != service.AccountTypeOAuth || len(account.Credentials) == 0 {
		return CLIProxyAuthFile{}, false
	}

	data := make(map[string]any)
	for k, v := range account.Credentials {
		data[k] = v
	}

	switch account.Platform {
	case service.PlatformOpenAI:
		data["type"] = "codex"
		if accountID := firstStringValue(data, "account_id", "chatgpt_account_id"); accountID != "" {
			data["account_id"] = accountID
		}
		if expiresAt := firstStringValue(data, "expired", "expires_at"); expiresAt != "" {
			data["expired"] = expiresAt
		}
	case service.PlatformGemini:
		data["type"] = "gemini"
		data["token"] = googleTokenMapFromCredentials(data)
	case service.PlatformAntigravity:
		data["type"] = "antigravity"
		data["token"] = googleTokenMapFromCredentials(data)
	default:
		return CLIProxyAuthFile{}, false
	}

	return CLIProxyAuthFile{
		Name: cliProxyAuthFileName(account),
		Data: data,
	}, true
}

func cliProxyAuthFileName(account service.Account) string {
	base := strings.TrimSpace(account.Name)
	if base == "" {
		base = fmt.Sprintf("account-%d", account.ID)
	}
	base = regexp.MustCompile(`[^a-zA-Z0-9._@-]+`).ReplaceAllString(base, "_")
	base = strings.Trim(base, "._-")
	if base == "" {
		base = fmt.Sprintf("account-%d", account.ID)
	}
	if !strings.HasSuffix(strings.ToLower(base), ".json") {
		base += ".json"
	}
	return base
}

func copyGoogleToken(dst map[string]any, src map[string]any) {
	if token, ok := src["token"].(map[string]any); ok {
		copyStringFields(dst, token, "access_token", "refresh_token", "token_type", "scope")
		setIfNotEmpty(dst, "expires_at", firstStringValue(token, "expiry", "expires_at"))
	}
	copyStringFields(dst, src, "access_token", "refresh_token", "token_type", "scope")
	setIfNotEmpty(dst, "expires_at", firstStringValue(src, "expires_at", "expired", "expiry"))
}

func googleTokenMapFromCredentials(credentials map[string]any) map[string]any {
	token := make(map[string]any)
	copyStringFields(token, credentials, "access_token", "refresh_token", "token_type", "scope")
	setIfNotEmpty(token, "expiry", firstStringValue(credentials, "expires_at", "expired", "expiry"))
	return token
}

func copyStringFields(dst map[string]any, src map[string]any, keys ...string) {
	for _, key := range keys {
		setIfNotEmpty(dst, key, stringValue(src, key))
	}
}

func setIfNotEmpty(dst map[string]any, key, value string) {
	if strings.TrimSpace(value) != "" {
		dst[key] = value
	}
}

func firstStringValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(data, key); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringValue(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	switch v := data[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}
