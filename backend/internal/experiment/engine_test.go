package experiment

import (
	"testing"
)

func testLayer() Layer {
	l := Layer{ID: 1, Key: "checkout", Name: "Checkout", HashNamespace: "growth.checkout", Salt: "2026-09-29"}
	for _, exp := range []struct {
		key        string
		start, end int
	}{
		{"paywall_v3", 0, 4999},
		{"checkout_banner", 5000, 7499},
	} {
		e := Experiment{
			ID:          int64(exp.start + 1),
			LayerID:     1,
			Key:         exp.key,
			Name:        exp.key,
			BucketStart: exp.start,
			BucketEnd:   exp.end,
			Active:      true,
			Targeting: Targeting{
				Countries:         []string{"US", "CA"},
				RequireRegistered: true,
				MinAccountAgeDays: 7,
			},
			Variants: []Variant{
				{Key: "control", Name: "control", Weight: 5000},
				{Key: "red", Name: "red", Weight: 2500},
				{Key: "blue", Name: "blue", Weight: 2500},
			},
		}
		l.Experiments = append(l.Experiments, e)
	}
	l.Experiments[0].Whitelist = []Whitelist{{ExperimentID: 1, UserID: "vip-001", VariantKey: "red", Reason: "VIP"}}
	return l
}

func TestBoundariesAreClosedIntervals(t *testing.T) {
	cases := map[string]string{
		"0":    "boundary-20133",
		"2499": "boundary-4961",
		"2500": "boundary-8281",
		"4999": "boundary-4284",
		"5000": "boundary-3792",
		"9999": "boundary-8518",
	}
	want := map[string]int{
		"0": 0, "2499": 2499, "2500": 2500, "4999": 4999, "5000": 5000, "9999": 9999,
	}
	for name, user := range cases {
		input := LayerHashInput("growth.checkout", "2026-09-29", user)
		if got := StableBucket(input); got != want[name] {
			t.Fatalf("%s: got bucket %d want %d", name, got, want[name])
		}
	}
}

func TestMutuallyExclusiveBoundaryOwnership(t *testing.T) {
	engine := NewEngine()
	layer := testLayer()
	cases := []struct {
		user       string
		experiment string
		status     string
	}{
		{"boundary-4961", "paywall_v3", "assigned_bucket"},
		{"boundary-4284", "paywall_v3", "assigned_bucket"},
		{"boundary-3792", "checkout_banner", "assigned_bucket"},
		{"boundary-8518", "", "out_of_traffic"},
	}
	for _, tc := range cases {
		d := engine.Evaluate(UserInput{UserID: tc.user, Known: true, Country: "US", Registered: true, AccountAgeDays: 30}, layer)
		if d.ExperimentKey != tc.experiment || d.Status != tc.status {
			t.Fatalf("%s: got experiment=%s status=%s want %s/%s; trace=%+v", tc.user, d.ExperimentKey, d.Status, tc.experiment, tc.status, d.Trace)
		}
	}
}

func TestUnknownIdentityRejectedBeforeHashOrExposure(t *testing.T) {
	engine := NewEngine()
	d := engine.Evaluate(UserInput{UserID: "unknown-person", Known: false, Country: "US", Registered: true, AccountAgeDays: 30}, testLayer())
	if d.Status != "rejected" || d.ReasonCode != "UNKNOWN_IDENTITY" {
		t.Fatalf("got status=%s reason=%s", d.Status, d.ReasonCode)
	}
	if d.Bucket != nil || d.HashInput != "" || d.ExposureSaved {
		t.Fatalf("unknown identity must not hash, allocate, or expose: %+v", d)
	}
}

func TestIneligibleUserAllocatedButNotAssignedOrExposed(t *testing.T) {
	engine := NewEngine()
	d := engine.Evaluate(UserInput{UserID: "synthetic-000001", Known: true, Country: "FR", Registered: true, AccountAgeDays: 30}, testLayer())
	if d.Status != "ineligible" || d.ReasonCode != "COUNTRY_NOT_ALLOWED" {
		t.Fatalf("got status=%s reason=%s", d.Status, d.ReasonCode)
	}
	if !d.Allocated || d.Assigned || d.ExposureSaved || d.VariantKey != "" {
		t.Fatalf("allocation can be visible, but no variant/exposure: %+v", d)
	}
}

func TestWhitelistOverridesAndIsSeparatelySourced(t *testing.T) {
	engine := NewEngine()
	d := engine.Evaluate(UserInput{UserID: "vip-001", Known: true, Country: "FR", Registered: false, AccountAgeDays: 0}, testLayer())
	if d.Status != "assigned_whitelist" || d.VariantKey != "red" || d.Source != "whitelist" {
		t.Fatalf("whitelist override failed: %+v", d)
	}
	if d.ReasonCode != "WHITELIST_OVERRIDE" || d.Reason == "" {
		t.Fatalf("override reason must be retained: %+v", d)
	}
}

func TestDeterminismAcrossSaltAndIdentity(t *testing.T) {
	inputA := LayerHashInput("growth.checkout", "2026-09-29", "synthetic-000001")
	inputB := LayerHashInput("growth.checkout", "2026-09-29", "synthetic-000001")
	inputSalted := LayerHashInput("growth.checkout", "2026-10-01", "synthetic-000001")
	inputOtherNS := LayerHashInput("other.namespace", "2026-09-29", "synthetic-000001")
	if StableBucket(inputA) != StableBucket(inputB) {
		t.Fatal("same input is not deterministic")
	}
	if StableBucket(inputA) == StableBucket(inputSalted) && StableBucket(inputA) == StableBucket(inputOtherNS) {
		t.Fatal("salt and namespace are not participating in hash input")
	}
}
