package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	DuplicateStrategy    string             `json:"duplicate_strategy"`
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

const (
	cliProxyAuthDuplicateStrategySkip    = "skip"
	cliProxyAuthDuplicateStrategyReplace = "replace"
)

type CLIProxyAuthDuplicateItem struct {
	ImportName          string `json:"import_name"`
	SourceFile          string `json:"source_file,omitempty"`
	Platform            string `json:"platform"`
	MatchedBy           string `json:"matched_by"`
	ExistingAccountID   int64  `json:"existing_account_id"`
	ExistingAccountName string `json:"existing_account_name"`
}

type CLIProxyAuthDuplicateCheckResult struct {
	Duplicates []CLIProxyAuthDuplicateItem `json:"duplicates"`
}

type cliProxyAuthDuplicateIdentity struct {
	Key   string
	Field string
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
	if strategy := normalizeCLIProxyAuthDuplicateStrategy(req.DuplicateStrategy); strategy == "" {
		response.BadRequest(c, "invalid duplicate_strategy")
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_cliproxy_auths", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importCLIProxyAuthData(ctx, req, dataReq)
	})
}

func (h *AccountHandler) CheckCLIProxyAuthDuplicates(c *gin.Context) {
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

	duplicates, _, err := h.findCLIProxyAuthDuplicates(c.Request.Context(), dataReq.Data.Accounts)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, CLIProxyAuthDuplicateCheckResult{
		Duplicates: duplicates,
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

func (h *AccountHandler) importCLIProxyAuthData(ctx context.Context, req CLIProxyAuthImportRequest, dataReq DataImportRequest) (DataImportResult, error) {
	skipDefaultGroupBind := true
	if req.SkipDefaultGroupBind != nil {
		skipDefaultGroupBind = *req.SkipDefaultGroupBind
	}

	result := DataImportResult{}

	existingProxies, err := h.listAllProxies(ctx)
	if err != nil {
		return result, err
	}

	proxyKeyToID := make(map[string]int64, len(existingProxies))
	for i := range existingProxies {
		p := existingProxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		proxyKeyToID[key] = p.ID
	}

	_, duplicateAccounts, err := h.findCLIProxyAuthDuplicates(ctx, dataReq.Data.Accounts)
	if err != nil {
		return result, err
	}

	duplicateStrategy := normalizeCLIProxyAuthDuplicateStrategy(req.DuplicateStrategy)
	if duplicateStrategy == "" {
		duplicateStrategy = cliProxyAuthDuplicateStrategySkip
	}

	duplicateByIndex := make(map[int]service.Account, len(duplicateAccounts))
	for index, account := range duplicateAccounts {
		duplicateByIndex[index] = account
	}

	var privacyAccounts []*service.Account
	for i := range dataReq.Data.Accounts {
		item := dataReq.Data.Accounts[i]
		if err := validateDataAccount(item); err != nil {
			result.AccountFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:    "account",
				Name:    item.Name,
				Message: err.Error(),
			})
			continue
		}

		var proxyID *int64
		if item.ProxyKey != nil && *item.ProxyKey != "" {
			if id, ok := proxyKeyToID[*item.ProxyKey]; ok {
				proxyID = &id
			} else {
				result.AccountFailed++
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "account",
					Name:     item.Name,
					ProxyKey: *item.ProxyKey,
					Message:  "proxy_key not found",
				})
				continue
			}
		}

		enrichCredentialsFromIDToken(&item)

		if existingAccount, ok := duplicateByIndex[i]; ok {
			if duplicateStrategy == cliProxyAuthDuplicateStrategyReplace {
				updated, updateErr := h.replaceCLIProxyAuthDuplicate(ctx, existingAccount, item, proxyID)
				if updateErr != nil {
					result.AccountFailed++
					result.Errors = append(result.Errors, DataImportError{
						Kind:    "account",
						Name:    item.Name,
						Message: updateErr.Error(),
					})
					continue
				}
				if updated != nil && updated.Platform == service.PlatformAntigravity && updated.Type == service.AccountTypeOAuth {
					privacyAccounts = append(privacyAccounts, updated)
				}
				result.AccountReplaced++
				continue
			}

			result.AccountSkipped++
			continue
		}

		accountInput := &service.CreateAccountInput{
			Name:                 item.Name,
			Notes:                item.Notes,
			Platform:             item.Platform,
			Type:                 item.Type,
			Credentials:          item.Credentials,
			Extra:                item.Extra,
			ProxyID:              proxyID,
			Concurrency:          item.Concurrency,
			Priority:             item.Priority,
			RateMultiplier:       item.RateMultiplier,
			GroupIDs:             nil,
			ExpiresAt:            item.ExpiresAt,
			AutoPauseOnExpired:   item.AutoPauseOnExpired,
			SkipDefaultGroupBind: skipDefaultGroupBind,
		}

		created, createErr := h.adminService.CreateAccount(ctx, accountInput)
		if createErr != nil {
			result.AccountFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:    "account",
				Name:    item.Name,
				Message: createErr.Error(),
			})
			continue
		}
		if created.Platform == service.PlatformAntigravity && created.Type == service.AccountTypeOAuth {
			privacyAccounts = append(privacyAccounts, created)
		}
		result.AccountCreated++
	}

	if len(privacyAccounts) > 0 {
		adminSvc := h.adminService
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("import_antigravity_privacy_panic", "recover", r)
				}
			}()
			bgCtx := context.Background()
			for _, acc := range privacyAccounts {
				adminSvc.ForceAntigravityPrivacy(bgCtx, acc)
			}
			slog.Info("import_antigravity_privacy_done", "count", len(privacyAccounts))
		}()
	}

	return result, nil
}

func (h *AccountHandler) findCLIProxyAuthDuplicates(ctx context.Context, importedAccounts []DataAccount) ([]CLIProxyAuthDuplicateItem, map[int]service.Account, error) {
	existingAccounts, err := h.listAccountsFiltered(ctx, "", "", "", "")
	if err != nil {
		return nil, nil, err
	}

	existingByIdentity := make(map[string]service.Account)
	for i := range existingAccounts {
		account := existingAccounts[i]
		if strings.TrimSpace(strings.ToLower(account.Type)) != service.AccountTypeOAuth {
			continue
		}
		for _, identity := range cliProxyAuthDuplicateIdentities(account.Platform, account.Credentials) {
			if identity.Key == "" {
				continue
			}
			if _, exists := existingByIdentity[identity.Key]; !exists {
				existingByIdentity[identity.Key] = account
			}
		}
	}

	duplicates := make([]CLIProxyAuthDuplicateItem, 0)
	duplicateAccounts := make(map[int]service.Account)
	for i := range importedAccounts {
		item := importedAccounts[i]
		match := cliProxyAuthDuplicateMatch{}
		for _, identity := range cliProxyAuthDuplicateIdentities(item.Platform, item.Credentials) {
			if identity.Key == "" {
				continue
			}
			existingAccount, ok := existingByIdentity[identity.Key]
			if !ok {
				continue
			}
			match = cliProxyAuthDuplicateMatch{
				Account: existingAccount,
				Field:   identity.Field,
			}
			break
		}
		if match.Account.ID == 0 {
			continue
		}
		duplicateAccounts[i] = match.Account
		duplicates = append(duplicates, CLIProxyAuthDuplicateItem{
			ImportName:          item.Name,
			SourceFile:          cliProxyAuthSourceFile(item),
			Platform:            item.Platform,
			MatchedBy:           match.Field,
			ExistingAccountID:   match.Account.ID,
			ExistingAccountName: match.Account.Name,
		})
	}

	return duplicates, duplicateAccounts, nil
}

func (h *AccountHandler) replaceCLIProxyAuthDuplicate(ctx context.Context, existingAccount service.Account, item DataAccount, proxyID *int64) (*service.Account, error) {
	updateInput := &service.UpdateAccountInput{
		Name:               item.Name,
		Notes:              item.Notes,
		Type:               item.Type,
		Credentials:        item.Credentials,
		RateMultiplier:     item.RateMultiplier,
		ExpiresAt:          item.ExpiresAt,
		AutoPauseOnExpired: item.AutoPauseOnExpired,
	}

	if proxyID != nil {
		updateInput.ProxyID = proxyID
	}

	concurrency := item.Concurrency
	priority := item.Priority
	updateInput.Concurrency = &concurrency
	updateInput.Priority = &priority

	if mergedExtra := mergeCLIProxyAuthExtra(existingAccount.Extra, item.Extra); mergedExtra != nil {
		updateInput.Extra = mergedExtra
	}

	return h.adminService.UpdateAccount(ctx, existingAccount.ID, updateInput)
}

func mergeCLIProxyAuthExtra(existing, imported map[string]any) map[string]any {
	if len(imported) == 0 {
		return nil
	}
	merged := make(map[string]any, len(existing)+len(imported))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range imported {
		merged[key] = value
	}
	return merged
}

func cliProxyAuthSourceFile(item DataAccount) string {
	if item.Extra == nil {
		return ""
	}
	return stringValue(item.Extra, "cliproxy_auth_file")
}

func normalizeCLIProxyAuthDuplicateStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", cliProxyAuthDuplicateStrategySkip:
		return cliProxyAuthDuplicateStrategySkip
	case cliProxyAuthDuplicateStrategyReplace:
		return cliProxyAuthDuplicateStrategyReplace
	default:
		return ""
	}
}

type cliProxyAuthDuplicateMatch struct {
	Account service.Account
	Field   string
}

func cliProxyAuthDuplicateIdentities(platform string, credentials map[string]any) []cliProxyAuthDuplicateIdentity {
	platform = strings.ToLower(strings.TrimSpace(platform))
	out := make([]cliProxyAuthDuplicateIdentity, 0, 5)
	appendIdentity := func(field string, values ...string) {
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			out = append(out, cliProxyAuthDuplicateIdentity{
				Key:   buildCLIProxyAuthDuplicateKey(platform, field, value),
				Field: field,
			})
			return
		}
	}
	switch platform {
	case service.PlatformOpenAI:
		appendIdentity("chatgpt_account_id", firstStringValue(credentials, "chatgpt_account_id", "account_id"))
		appendIdentity("email", firstStringValue(credentials, "email"))
		appendIdentity("chatgpt_user_id", firstStringValue(credentials, "chatgpt_user_id"))
	case service.PlatformGemini, service.PlatformAntigravity:
		projectID := firstStringValue(credentials, "project_id")
		if projectID != "" {
			appendIdentity("project_id", projectID)
		} else {
			appendIdentity("email", firstStringValue(credentials, "email"))
		}
	default:
		appendIdentity("email", firstStringValue(credentials, "email"))
	}

	appendIdentity("refresh_token", firstStringValue(credentials, "refresh_token"))
	appendIdentity("access_token", firstStringValue(credentials, "access_token"))

	seen := make(map[string]struct{}, len(out))
	deduped := make([]cliProxyAuthDuplicateIdentity, 0, len(out))
	for _, identity := range out {
		if _, exists := seen[identity.Key]; exists {
			continue
		}
		seen[identity.Key] = struct{}{}
		deduped = append(deduped, identity)
	}
	return deduped
}

func buildCLIProxyAuthDuplicateKey(platform, field, value string) string {
	normalizedValue := strings.TrimSpace(value)
	if field != "refresh_token" && field != "access_token" {
		normalizedValue = strings.ToLower(normalizedValue)
	}
	return platform + "|" + field + "|" + normalizedValue
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
