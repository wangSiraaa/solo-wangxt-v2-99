package db

import (
	"context"

	"github.com/example/explainab/internal/experiment"
)

func DefaultLayers() []experiment.Layer {
	return []experiment.Layer{
		{
			Key:           "checkout",
			Name:          "Checkout monetization mutex layer",
			HashNamespace: "growth.checkout",
			Salt:          "2026-09-29",
			Experiments: []experiment.Experiment{
				{
					Key:         "paywall_v3",
					Name:        "Paywall redesign",
					BucketStart: 0,
					BucketEnd:   4999,
					Active:      true,
					Targeting: experiment.Targeting{
						Countries:         []string{"US", "CA"},
						RequireRegistered: true,
						MinAccountAgeDays: 7,
					},
					Variants: []experiment.Variant{
						{Key: "control", Name: "Current paywall", Weight: 5000},
						{Key: "red", Name: "Red primary CTA", Weight: 2500},
						{Key: "blue", Name: "Blue primary CTA", Weight: 2500},
					},
					Whitelist: []experiment.Whitelist{
						{UserID: "vip-001", VariantKey: "red", Reason: "Customer success override for VIP design preview"},
					},
				},
				{
					Key:         "checkout_banner",
					Name:        "Checkout trust banner",
					BucketStart: 5000,
					BucketEnd:   7499,
					Active:      true,
					Targeting: experiment.Targeting{
						Countries:         []string{"US", "CA", "FR"},
						RequireRegistered: true,
						MinAccountAgeDays: 3,
					},
					Variants: []experiment.Variant{
						{Key: "control", Name: "No banner", Weight: 5000},
						{Key: "badges", Name: "Security badges", Weight: 2500},
						{Key: "reviews", Name: "Review excerpts", Weight: 2500},
					},
					Whitelist: []experiment.Whitelist{
						{UserID: "force-banner", VariantKey: "badges", Reason: "Operations demo override for banner badges"},
					},
				},
			},
		},
	}
}

func SeedDefaults(ctx context.Context, repo *Repo) error {
	return repo.Seed(ctx, DefaultLayers())
}
