package handler

import (
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type AvailableChannelHandler struct {
	channelService *service.ChannelService
	apiKeyService  *service.APIKeyService
}

func NewAvailableChannelHandler(
	channelService *service.ChannelService,
	apiKeyService *service.APIKeyService,
) *AvailableChannelHandler {
	return &AvailableChannelHandler{
		channelService: channelService,
		apiKeyService:  apiKeyService,
	}
}

type userAvailableGroup struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Platform         string  `json:"platform"`
	SubscriptionType string  `json:"subscription_type"`
	RateMultiplier   float64 `json:"rate_multiplier"`
	IsExclusive      bool    `json:"is_exclusive"`
}

type userPricingIntervalDTO struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       string   `json:"tier_label,omitempty"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
}

type userSupportedModelPricing struct {
	BillingMode      string                   `json:"billing_mode"`
	InputPrice       *float64                 `json:"input_price"`
	OutputPrice      *float64                 `json:"output_price"`
	CacheWritePrice  *float64                 `json:"cache_write_price"`
	CacheReadPrice   *float64                 `json:"cache_read_price"`
	ImageOutputPrice *float64                 `json:"image_output_price"`
	PerRequestPrice  *float64                 `json:"per_request_price"`
	Intervals        []userPricingIntervalDTO `json:"intervals"`
}

type userSupportedModel struct {
	Name     string                     `json:"name"`
	Platform string                     `json:"platform"`
	Pricing  *userSupportedModelPricing `json:"pricing"`
}

type userChannelPlatformSection struct {
	Platform        string               `json:"platform"`
	Groups          []userAvailableGroup `json:"groups"`
	SupportedModels []userSupportedModel `json:"supported_models"`
}

type userAvailableChannel struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Platforms   []userChannelPlatformSection `json:"platforms"`
}

func (h *AvailableChannelHandler) List(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	userGroups, err := h.apiKeyService.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	allowedGroupIDs := make(map[int64]struct{}, len(userGroups))
	for i := range userGroups {
		allowedGroupIDs[userGroups[i].ID] = struct{}{}
	}

	channels, err := h.channelService.ListAvailable(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]userAvailableChannel, 0, len(channels))
	for _, channel := range channels {
		if channel.Status != service.StatusActive {
			continue
		}
		visibleGroups := filterUserVisibleGroups(channel.Groups, allowedGroupIDs)
		if len(visibleGroups) == 0 {
			continue
		}
		sections := buildPlatformSections(channel, visibleGroups)
		if len(sections) == 0 {
			continue
		}
		out = append(out, userAvailableChannel{
			Name:        channel.Name,
			Description: channel.Description,
			Platforms:   sections,
		})
	}

	response.Success(c, out)
}

func buildPlatformSections(channel service.AvailableChannel, visibleGroups []userAvailableGroup) []userChannelPlatformSection {
	groupsByPlatform := make(map[string][]userAvailableGroup, 4)
	for _, group := range visibleGroups {
		if group.Platform == "" {
			continue
		}
		groupsByPlatform[group.Platform] = append(groupsByPlatform[group.Platform], group)
	}
	if len(groupsByPlatform) == 0 {
		return nil
	}

	platforms := make([]string, 0, len(groupsByPlatform))
	for platform := range groupsByPlatform {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)

	sections := make([]userChannelPlatformSection, 0, len(platforms))
	for _, platform := range platforms {
		allowedPlatforms := map[string]struct{}{platform: {}}
		sections = append(sections, userChannelPlatformSection{
			Platform:        platform,
			Groups:          groupsByPlatform[platform],
			SupportedModels: toUserSupportedModels(channel.SupportedModels, allowedPlatforms),
		})
	}

	return sections
}

func filterUserVisibleGroups(groups []service.AvailableGroupRef, allowed map[int64]struct{}) []userAvailableGroup {
	visible := make([]userAvailableGroup, 0, len(groups))
	for _, group := range groups {
		if _, ok := allowed[group.ID]; !ok {
			continue
		}
		visible = append(visible, userAvailableGroup{
			ID:               group.ID,
			Name:             group.Name,
			Platform:         group.Platform,
			SubscriptionType: group.SubscriptionType,
			RateMultiplier:   group.RateMultiplier,
			IsExclusive:      group.IsExclusive,
		})
	}
	return visible
}

func toUserSupportedModels(src []service.SupportedModel, allowedPlatforms map[string]struct{}) []userSupportedModel {
	out := make([]userSupportedModel, 0, len(src))
	for i := range src {
		model := src[i]
		if allowedPlatforms != nil {
			if _, ok := allowedPlatforms[model.Platform]; !ok {
				continue
			}
		}
		out = append(out, userSupportedModel{
			Name:     model.Name,
			Platform: model.Platform,
			Pricing:  toUserPricing(model.Pricing),
		})
	}
	return out
}

func toUserPricing(pricing *service.ChannelModelPricing) *userSupportedModelPricing {
	if pricing == nil {
		return nil
	}

	intervals := make([]userPricingIntervalDTO, 0, len(pricing.Intervals))
	for _, interval := range pricing.Intervals {
		intervals = append(intervals, userPricingIntervalDTO{
			MinTokens:       interval.MinTokens,
			MaxTokens:       interval.MaxTokens,
			TierLabel:       interval.TierLabel,
			InputPrice:      interval.InputPrice,
			OutputPrice:     interval.OutputPrice,
			CacheWritePrice: interval.CacheWritePrice,
			CacheReadPrice:  interval.CacheReadPrice,
			PerRequestPrice: interval.PerRequestPrice,
		})
	}

	billingMode := string(pricing.BillingMode)
	if billingMode == "" {
		billingMode = string(service.BillingModeToken)
	}

	return &userSupportedModelPricing{
		BillingMode:      billingMode,
		InputPrice:       pricing.InputPrice,
		OutputPrice:      pricing.OutputPrice,
		CacheWritePrice:  pricing.CacheWritePrice,
		CacheReadPrice:   pricing.CacheReadPrice,
		ImageOutputPrice: pricing.ImageOutputPrice,
		PerRequestPrice:  pricing.PerRequestPrice,
		Intervals:        intervals,
	}
}
