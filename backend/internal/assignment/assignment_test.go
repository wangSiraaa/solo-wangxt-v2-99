package assignment

import (
	"testing"

	"github.com/example/ab-platform/internal/storage"
)

func subject(id, country, plan string, registered bool) storage.Subject {
	return storage.Subject{UserID: id, Registered: registered, Country: country, Plan: plan, Source: "synthetic"}
}

func TestStableAcrossRestartsAndOrder(t *testing.T) {
	first, err := Evaluate(storage.SeedConfig(), subject("syn-3340", "US", "free", true), "checkout_layer")
	if err != nil {
		t.Fatal(err)
	}
	// Re-building SeedConfig simulates a fresh process. The explicit hash
	// formula means no assignment is stored in process memory.
	second, err := Evaluate(storage.SeedConfig(), subject("syn-3340", "US", "free", true), "checkout_layer")
	if err != nil {
		t.Fatal(err)
	}
	if first.HashInput != second.HashInput || first.Bucket != second.Bucket ||
		first.Experiment.ID != second.Experiment.ID || first.Variant.ID != second.Variant.ID {
		t.Fatalf("decision changed: %+v vs %+v", first, second)
	}
	if first.HashInput != "growth:checkout_layer:layer-salt-v1:syn-3340" || first.Bucket != 4999 {
		t.Fatalf("unexpected hash path: %s %d", first.HashInput, first.Bucket)
	}
	if first.Experiment.ID != "one_click_promo" || first.Variant.ID != "one_click_control" {
		t.Fatalf("expected promo control from variant bucket %d, got %s/%s", first.VariantBucket, first.Experiment.ID, first.Variant.ID)
	}
}

func TestMutuallyExclusiveIntervalsAreHalfOpen(t *testing.T) {
	cfg := storage.SeedConfig()
	left, err := Evaluate(cfg, subject("syn-3340", "US", "free", true), "checkout_layer")
	if err != nil {
		t.Fatal(err)
	}
	right, err := Evaluate(storage.SeedConfig(), subject("syn-15461", "US", "paid", true), "checkout_layer")
	if err != nil {
		t.Fatal(err)
	}
	if left.Bucket != 4999 || left.Experiment.ID != "one_click_promo" {
		t.Fatalf("left boundary got bucket=%d exp=%v", left.Bucket, left.Experiment)
	}
	if right.Bucket != 5000 || right.Experiment.ID != "rewards_panel" {
		t.Fatalf("right boundary got bucket=%d exp=%v", right.Bucket, right.Experiment)
	}
}

func TestUnknownIdentityRejectedWithoutVariantOrExposureEligibility(t *testing.T) {
	d, err := Evaluate(storage.SeedConfig(), subject("unknown-user-x", "US", "paid", false), "checkout_layer")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusRejected || d.RejectReason != ReasonUnknownIdentity {
		t.Fatalf("got status=%s reason=%s", d.Status, d.RejectReason)
	}
	if d.Experiment != nil || d.Variant != nil {
		t.Fatalf("unknown identity must not select experiment/variant: %+v", d)
	}
}

func TestIneligibleHashHitRejectedAndDoesNotTrySecondExperiment(t *testing.T) {
	// syn-15461 deterministically enters the rewards half, but free plan is
	// not eligible. Mutex exclusion must not silently move them to promo.
	d, err := Evaluate(storage.SeedConfig(), subject("syn-15461", "US", "free", true), "checkout_layer")
	if err != nil {
		t.Fatal(err)
	}
	if d.Experiment.ID != "rewards_panel" || d.Status != StatusRejected || d.RejectReason != ReasonPlanBlocked {
		t.Fatalf("got exp=%v status=%s reason=%s", d.Experiment, d.Status, d.RejectReason)
	}
}

func TestWhitelistOverridesHashAndRecordsReason(t *testing.T) {
	d, err := Evaluate(storage.SeedConfig(), subject("vip-001", "CA", "free", true), "checkout_layer")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusAssigned || d.Whitelist == nil || d.Experiment.ID != "rewards_panel" ||
		d.Variant.ID != "rewards_treatment" {
		t.Fatalf("whitelist override failed: %+v", d)
	}
	if d.Whitelist.Reason == "" {
		t.Fatal("override reason must be retained for separately labeled stats")
	}
}
