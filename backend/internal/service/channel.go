package service

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type BillingMode string

const (
	BillingModeToken      BillingMode = "token"
	BillingModePerRequest BillingMode = "per_request"
	BillingModeImage      BillingMode = "image"
)

func (m BillingMode) IsValid() bool {
	switch m {
	case BillingModeToken, BillingModePerRequest, BillingModeImage, "":
		return true
	}
	return false
}

const (
	BillingModelSourceRequested     = "requested"
	BillingModelSourceUpstream      = "upstream"
	BillingModelSourceChannelMapped = "channel_mapped"
)

type Channel struct {
	ID                 int64
	Name               string
	Description        string
	Status             string
	BillingModelSource string
	RestrictModels     bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	GroupIDs           []int64
	ModelPricing       []ChannelModelPricing
	ModelMapping       map[string]map[string]string
}

type ChannelModelPricing struct {
	ID               int64
	ChannelID        int64
	Platform         string
	Models           []string
	BillingMode      BillingMode
	InputPrice       *float64
	OutputPrice      *float64
	CacheWritePrice  *float64
	CacheReadPrice   *float64
	ImageOutputPrice *float64
	PerRequestPrice  *float64
	Intervals        []PricingInterval
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type PricingInterval struct {
	ID              int64
	PricingID       int64
	MinTokens       int
	MaxTokens       *int
	TierLabel       string
	InputPrice      *float64
	OutputPrice     *float64
	CacheWritePrice *float64
	CacheReadPrice  *float64
	PerRequestPrice *float64
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (c *Channel) IsActive() bool {
	return c.Status == StatusActive
}

func (c *Channel) normalizeBillingModelSource() {
	if c == nil {
		return
	}
	if c.BillingModelSource == "" {
		c.BillingModelSource = BillingModelSourceChannelMapped
	}
}

func (c *Channel) GetModelPricing(model string) *ChannelModelPricing {
	modelLower := strings.ToLower(model)

	for i := range c.ModelPricing {
		for _, name := range c.ModelPricing[i].Models {
			if strings.ToLower(name) == modelLower {
				cp := c.ModelPricing[i].Clone()
				return &cp
			}
		}
	}

	return nil
}

func FindMatchingInterval(intervals []PricingInterval, totalTokens int) *PricingInterval {
	for i := range intervals {
		interval := &intervals[i]
		if totalTokens > interval.MinTokens && (interval.MaxTokens == nil || totalTokens <= *interval.MaxTokens) {
			return interval
		}
	}
	return nil
}

func (p *ChannelModelPricing) GetIntervalForContext(totalTokens int) *PricingInterval {
	return FindMatchingInterval(p.Intervals, totalTokens)
}

func (p *ChannelModelPricing) GetTierByLabel(label string) *PricingInterval {
	labelLower := strings.ToLower(label)
	for i := range p.Intervals {
		if strings.ToLower(p.Intervals[i].TierLabel) == labelLower {
			return &p.Intervals[i]
		}
	}
	return nil
}

func (p ChannelModelPricing) Clone() ChannelModelPricing {
	cp := p
	if p.Models != nil {
		cp.Models = make([]string, len(p.Models))
		copy(cp.Models, p.Models)
	}
	if p.Intervals != nil {
		cp.Intervals = make([]PricingInterval, len(p.Intervals))
		copy(cp.Intervals, p.Intervals)
	}
	return cp
}

func (c *Channel) Clone() *Channel {
	if c == nil {
		return nil
	}
	cp := *c
	if c.GroupIDs != nil {
		cp.GroupIDs = make([]int64, len(c.GroupIDs))
		copy(cp.GroupIDs, c.GroupIDs)
	}
	if c.ModelPricing != nil {
		cp.ModelPricing = make([]ChannelModelPricing, len(c.ModelPricing))
		for i := range c.ModelPricing {
			cp.ModelPricing[i] = c.ModelPricing[i].Clone()
		}
	}
	if c.ModelMapping != nil {
		cp.ModelMapping = make(map[string]map[string]string, len(c.ModelMapping))
		for platform, mapping := range c.ModelMapping {
			inner := make(map[string]string, len(mapping))
			for key, value := range mapping {
				inner[key] = value
			}
			cp.ModelMapping[platform] = inner
		}
	}
	return &cp
}

func ValidateIntervals(intervals []PricingInterval) error {
	if len(intervals) == 0 {
		return nil
	}
	sorted := make([]PricingInterval, len(intervals))
	copy(sorted, intervals)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].MinTokens < sorted[j].MinTokens
	})

	for i := range sorted {
		if err := validateSingleInterval(&sorted[i], i); err != nil {
			return err
		}
	}
	return validateIntervalOverlap(sorted)
}

func validateSingleInterval(interval *PricingInterval, idx int) error {
	if interval.MinTokens < 0 {
		return fmt.Errorf("interval #%d: min_tokens (%d) must be >= 0", idx+1, interval.MinTokens)
	}
	if interval.MaxTokens != nil {
		if *interval.MaxTokens <= 0 {
			return fmt.Errorf("interval #%d: max_tokens (%d) must be > 0", idx+1, *interval.MaxTokens)
		}
		if *interval.MaxTokens <= interval.MinTokens {
			return fmt.Errorf(
				"interval #%d: max_tokens (%d) must be > min_tokens (%d)",
				idx+1,
				*interval.MaxTokens,
				interval.MinTokens,
			)
		}
	}
	return validateIntervalPrices(interval, idx)
}

func validateIntervalPrices(interval *PricingInterval, idx int) error {
	prices := []struct {
		name string
		val  *float64
	}{
		{"input_price", interval.InputPrice},
		{"output_price", interval.OutputPrice},
		{"cache_write_price", interval.CacheWritePrice},
		{"cache_read_price", interval.CacheReadPrice},
		{"per_request_price", interval.PerRequestPrice},
	}
	for _, price := range prices {
		if price.val != nil && *price.val < 0 {
			return fmt.Errorf("interval #%d: %s must be >= 0", idx+1, price.name)
		}
	}
	return nil
}

func validateIntervalOverlap(sorted []PricingInterval) error {
	for i, interval := range sorted {
		if interval.MaxTokens == nil && i < len(sorted)-1 {
			return fmt.Errorf("interval #%d: unbounded interval (max_tokens=null) must be the last one", i+1)
		}
		if i == 0 {
			continue
		}
		prev := sorted[i-1]
		if prev.MaxTokens == nil || *prev.MaxTokens > interval.MinTokens {
			return fmt.Errorf(
				"interval #%d and #%d overlap: prev max=%s > cur min=%d",
				i,
				i+1,
				formatMaxTokensLabel(prev.MaxTokens),
				interval.MinTokens,
			)
		}
	}
	return nil
}

func formatMaxTokensLabel(max *int) string {
	if max == nil {
		return "∞"
	}
	return fmt.Sprintf("%d", *max)
}

type ChannelUsageFields struct {
	ChannelID          int64
	OriginalModel      string
	ChannelMappedModel string
	BillingModelSource string
	ModelMappingChain  string
}

type SupportedModel struct {
	Name     string
	Platform string
	Pricing  *ChannelModelPricing
}

const wildcardSuffix = "*"

func splitWildcardSuffix(pattern string) (string, bool) {
	if strings.HasSuffix(pattern, wildcardSuffix) {
		return strings.TrimSuffix(pattern, wildcardSuffix), true
	}
	return pattern, false
}

func (c *Channel) GetModelPricingByPlatform(platform, model string) *ChannelModelPricing {
	if c == nil {
		return nil
	}
	modelLower := strings.ToLower(model)
	for i := range c.ModelPricing {
		if c.ModelPricing[i].Platform != platform {
			continue
		}
		for _, name := range c.ModelPricing[i].Models {
			if strings.ToLower(name) == modelLower {
				cp := c.ModelPricing[i].Clone()
				return &cp
			}
		}
	}
	return nil
}

type platformPricingIndex struct {
	byLower      map[string]*ChannelModelPricing
	originalCase map[string]string
	names        []string
}

func buildPricingIndex(pricings []ChannelModelPricing) map[string]*platformPricingIndex {
	indexes := make(map[string]*platformPricingIndex)
	for i := range pricings {
		pricing := pricings[i]
		index, ok := indexes[pricing.Platform]
		if !ok {
			index = &platformPricingIndex{
				byLower:      make(map[string]*ChannelModelPricing),
				originalCase: make(map[string]string),
				names:        make([]string, 0),
			}
			indexes[pricing.Platform] = index
		}
		for _, model := range pricing.Models {
			if _, wildcard := splitWildcardSuffix(model); wildcard {
				continue
			}
			lower := strings.ToLower(model)
			if _, exists := index.byLower[lower]; exists {
				continue
			}
			cp := pricings[i].Clone()
			index.byLower[lower] = &cp
			index.originalCase[lower] = model
			index.names = append(index.names, model)
		}
	}
	return indexes
}

func (c *Channel) SupportedModels() []SupportedModel {
	if c == nil {
		return nil
	}
	if len(c.ModelMapping) == 0 && len(c.ModelPricing) == 0 {
		return nil
	}

	indexes := buildPricingIndex(c.ModelPricing)

	type dedupKey struct {
		platform string
		name     string
	}

	seen := make(map[dedupKey]struct{})
	result := make([]SupportedModel, 0)

	lookup := func(index *platformPricingIndex, name string) (string, *ChannelModelPricing) {
		if index == nil || name == "" {
			return name, nil
		}
		lower := strings.ToLower(name)
		if pricing, ok := index.byLower[lower]; ok {
			return index.originalCase[lower], pricing
		}
		return name, nil
	}

	add := func(platform, displayName string, pricing *ChannelModelPricing) {
		if displayName == "" {
			return
		}
		key := dedupKey{platform: platform, name: strings.ToLower(displayName)}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, SupportedModel{
			Name:     displayName,
			Platform: platform,
			Pricing:  pricing,
		})
	}

	for platform, mapping := range c.ModelMapping {
		if len(mapping) == 0 {
			continue
		}
		index := indexes[platform]
		for source, target := range mapping {
			prefix, wildcard := splitWildcardSuffix(source)
			if wildcard {
				if index == nil {
					continue
				}
				prefixLower := strings.ToLower(prefix)
				for _, candidate := range index.names {
					if strings.HasPrefix(strings.ToLower(candidate), prefixLower) {
						display, pricing := lookup(index, candidate)
						add(platform, display, pricing)
					}
				}
				continue
			}

			pricingKey := target
			if pricingKey == "" {
				pricingKey = source
			}
			if _, wildcard := splitWildcardSuffix(pricingKey); wildcard {
				pricingKey = source
			}

			_, pricing := lookup(index, pricingKey)
			displayName, _ := lookup(index, source)
			add(platform, displayName, pricing)
		}
	}

	for platform, index := range indexes {
		for _, name := range index.names {
			display, pricing := lookup(index, name)
			add(platform, display, pricing)
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Platform != result[j].Platform {
			return result[i].Platform < result[j].Platform
		}
		return result[i].Name < result[j].Name
	})

	return result
}
