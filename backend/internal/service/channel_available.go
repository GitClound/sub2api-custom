package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type AvailableGroupRef struct {
	ID               int64
	Name             string
	Platform         string
	SubscriptionType string
	RateMultiplier   float64
	IsExclusive      bool
}

type AvailableChannel struct {
	ID                 int64
	Name               string
	Description        string
	Status             string
	BillingModelSource string
	RestrictModels     bool
	Groups             []AvailableGroupRef
	SupportedModels    []SupportedModel
}

func (s *ChannelService) ListAvailable(ctx context.Context) ([]AvailableChannel, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	if s.groupRepo == nil {
		return nil, fmt.Errorf("group repository not configured")
	}

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	groupByID := make(map[int64]AvailableGroupRef, len(groups))
	for i := range groups {
		group := groups[i]
		groupByID[group.ID] = AvailableGroupRef{
			ID:               group.ID,
			Name:             group.Name,
			Platform:         group.Platform,
			SubscriptionType: group.SubscriptionType,
			RateMultiplier:   group.RateMultiplier,
			IsExclusive:      group.IsExclusive,
		}
	}

	out := make([]AvailableChannel, 0, len(channels))
	for i := range channels {
		channel := &channels[i]
		refs := make([]AvailableGroupRef, 0, len(channel.GroupIDs))
		for _, groupID := range channel.GroupIDs {
			if ref, ok := groupByID[groupID]; ok {
				refs = append(refs, ref)
			}
		}
		sort.SliceStable(refs, func(i, j int) bool {
			return strings.ToLower(refs[i].Name) < strings.ToLower(refs[j].Name)
		})

		channel.normalizeBillingModelSource()
		supportedModels := channel.SupportedModels()
		s.fillGlobalPricingFallback(supportedModels)

		out = append(out, AvailableChannel{
			ID:                 channel.ID,
			Name:               channel.Name,
			Description:        channel.Description,
			Status:             channel.Status,
			BillingModelSource: channel.BillingModelSource,
			RestrictModels:     channel.RestrictModels,
			Groups:             refs,
			SupportedModels:    supportedModels,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	return out, nil
}

func (s *ChannelService) fillGlobalPricingFallback(models []SupportedModel) {
	if s.pricingService == nil {
		return
	}
	for i := range models {
		if models[i].Pricing != nil {
			continue
		}
		pricing := s.pricingService.GetModelPricing(models[i].Name)
		if pricing == nil {
			continue
		}
		models[i].Pricing = synthesizePricingFromLiteLLM(pricing)
	}
}

func synthesizePricingFromLiteLLM(pricing *LiteLLMModelPricing) *ChannelModelPricing {
	if pricing == nil {
		return nil
	}
	return &ChannelModelPricing{
		BillingMode:      BillingModeToken,
		InputPrice:       nonZeroPtr(pricing.InputCostPerToken),
		OutputPrice:      nonZeroPtr(pricing.OutputCostPerToken),
		CacheWritePrice:  nonZeroPtr(pricing.CacheCreationInputTokenCost),
		CacheReadPrice:   nonZeroPtr(pricing.CacheReadInputTokenCost),
		ImageOutputPrice: nonZeroPtr(pricing.OutputCostPerImageToken),
	}
}

func nonZeroPtr(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}
