package experiment

import (
	"fmt"
	"sort"
	"strings"
)

type AssignmentRecord struct {
	LayerID      int64
	ExperimentID int64
	Bucket       int
	VariantKey   string
	Source       string
	Reason       string
}

type Engine struct{}

func NewEngine() Engine { return Engine{} }

func intPtr(v int) *int { return &v }

func (e Engine) Evaluate(user UserInput, layer Layer) Decision {
	trace := []Step{}
	d := Decision{
		UserID:   user.UserID,
		LayerKey: layer.Key,
		Status:   "unallocated",
		Trace:    trace,
	}

	add := func(step Step) {
		d.Trace = append(d.Trace, step)
	}

	add(Step{Stage: "input", Status: "ok", Detail: fmt.Sprintf("Validated synthetic user %q", user.UserID)})
	if !user.Known {
		d.Eligible = false
		d.Allocated = false
		d.Assigned = false
		d.Status = "rejected"
		d.ReasonCode = "UNKNOWN_IDENTITY"
		d.Reason = "identity provider has no record for this user; no namespace hash, experiment evaluation, assignment, or exposure is created"
		add(Step{
			Stage:      "identity",
			Status:     "reject",
			ReasonCode: "UNKNOWN_IDENTITY",
			Detail:     "Unknown identity: deterministic bucketing would be possible, but this platform refuses to expose a treatment to an unverified principal.",
		})
		return d
	}
	add(Step{Stage: "identity", Status: "pass", Detail: "Identity is known; proceeding with deterministic layer evaluation."})

	experiments := append([]Experiment(nil), layer.Experiments...)
	sort.Slice(experiments, func(i, j int) bool { return experiments[i].BucketStart < experiments[j].BucketStart })

	for i := range experiments {
		exp := &experiments[i]
		if !exp.Active {
			continue
		}
		for _, wl := range exp.Whitelist {
			if wl.UserID == user.UserID {
				variant := findVariant(exp.Variants, wl.VariantKey)
				if variant == nil {
					continue
				}
				bucket := StableBucket(LayerHashInput(layer.HashNamespace, layer.Salt, user.UserID))
				variantInput := VariantHashInput(layer.HashNamespace, layer.Salt, exp.Key, user.UserID)
				d.HashInput = LayerHashInput(layer.HashNamespace, layer.Salt, user.UserID)
				d.Bucket = intPtr(bucket)
				d.ExperimentKey = exp.Key
				d.ExperimentName = exp.Name
				d.VariantKey = variant.Key
				d.Source = "whitelist"
				d.ReasonCode = "WHITELIST_OVERRIDE"
				d.Reason = wl.Reason
				d.Eligible = true
				d.Allocated = true
				d.Assigned = true
				d.Status = "assigned_whitelist"
				d.VariantRanges = buildVariantRanges(exp.Variants)
				add(Step{Stage: "whitelist", Status: "override", ExperimentKey: exp.Key, VariantKey: variant.Key, ReasonCode: "WHITELIST_OVERRIDE", Detail: fmt.Sprintf("Whitelist takes precedence: %s.", wl.Reason)})
				add(Step{Stage: "layer_bucket", Status: "informational", ExperimentKey: exp.Key, HashInput: d.HashInput, Bucket: &bucket, RangeStart: &exp.BucketStart, RangeEnd: &exp.BucketEnd, Detail: "The layer bucket is shown for explanation even though the whitelist bypasses range and targeting gates."})
				add(Step{Stage: "variant", Status: "whitelist", ExperimentKey: exp.Key, VariantKey: variant.Key, HashInput: variantInput, Detail: "Whitelist selects the variant directly; the variant hash is not used."})
				return d
			}
		}
	}

	hashInput := LayerHashInput(layer.HashNamespace, layer.Salt, user.UserID)
	bucket := StableBucket(hashInput)
	d.HashInput = hashInput
	d.Bucket = &bucket
	add(Step{Stage: "layer_bucket", Status: "hash", HashInput: hashInput, Bucket: &bucket, Detail: fmt.Sprintf("FNV-1a 64-bit hash mod 10000. Namespace=%q, salt=%q, stable user id=%q", layer.HashNamespace, layer.Salt, user.UserID)})

	var selected *Experiment
	for i := range experiments {
		exp := &experiments[i]
		if !exp.Active {
			continue
		}
		inside := bucket >= exp.BucketStart && bucket <= exp.BucketEnd
		add(Step{
			Stage:         "mutex_range",
			Status:        boolStatus(inside, "hit", "miss"),
			ExperimentKey: exp.Key,
			Bucket:        &bucket,
			RangeStart:    &exp.BucketStart,
			RangeEnd:      &exp.BucketEnd,
			Detail:        fmt.Sprintf("Closed interval [%d,%d] compared with layer bucket %d; intervals are disjoint and selected by bucket, not request order.", exp.BucketStart, exp.BucketEnd, bucket),
		})
		if inside {
			selected = exp
			break
		}
	}
	if selected == nil {
		d.Status = "out_of_traffic"
		d.ReasonCode = "NO_EXPERIMENT_RANGE"
		d.Reason = fmt.Sprintf("bucket %d does not land in any active experiment interval in layer %s", bucket, layer.Key)
		add(Step{Stage: "allocation", Status: "reject", ReasonCode: "NO_EXPERIMENT_RANGE", Bucket: &bucket, Detail: "No mutually exclusive experiment owns this bucket."})
		return d
	}
	d.Allocated = true
	d.ExperimentKey = selected.Key
	d.ExperimentName = selected.Name
	add(Step{Stage: "allocation", Status: "allocated", ExperimentKey: selected.Key, Bucket: &bucket, RangeStart: &selected.BucketStart, RangeEnd: &selected.BucketEnd, Detail: "The owning experiment is uniquely determined by the layer interval."})

	ok, code, reason := checkTargeting(user, selected.Targeting)
	if !ok {
		d.Eligible = false
		d.Status = "ineligible"
		d.ReasonCode = code
		d.Reason = reason
		add(Step{Stage: "targeting", Status: "reject", ExperimentKey: selected.Key, ReasonCode: code, Detail: reason})
		add(Step{Stage: "exposure", Status: "skip", ExperimentKey: selected.Key, ReasonCode: "EXPOSURE_NOT_RECORDED", Detail: "Eligibility failed; no assignment or exposure is recorded."})
		return d
	}
	d.Eligible = true
	add(Step{Stage: "targeting", Status: "pass", ExperimentKey: selected.Key, Detail: formatTargeting(selected.Targeting)})

	vhashInput := VariantHashInput(layer.HashNamespace, layer.Salt, selected.Key, user.UserID)
	vbucket := StableBucket(vhashInput)
	d.VariantRanges = buildVariantRanges(selected.Variants)
	variant := chooseVariant(selected.Variants, vbucket)
	add(Step{Stage: "variant_hash", Status: "hash", ExperimentKey: selected.Key, HashInput: vhashInput, Bucket: &vbucket, Detail: "A second stable hash under the experiment key keeps variant proportions independent from the layer interval."})
	if variant == nil {
		d.Status = "misconfigured"
		d.ReasonCode = "NO_VARIANT_BUCKET"
		d.Reason = "variant weights do not cover bucket 0-9999"
		return d
	}
	d.Assigned = true
	d.VariantKey = variant.Key
	d.Source = "bucket"
	d.Status = "assigned_bucket"
	add(Step{Stage: "variant", Status: "assigned", ExperimentKey: selected.Key, VariantKey: variant.Key, Bucket: &vbucket, Detail: fmt.Sprintf("Variant %s owns local variant bucket %d.", variant.Key, vbucket)})
	add(Step{Stage: "exposure", Status: boolStatus(user.RecordExposure, "record", "dry_run"), ExperimentKey: selected.Key, VariantKey: variant.Key, Detail: "Persisted exposures are written only when the caller requests production recording."})
	return d
}

func boolStatus(v bool, yes, no string) string {
	if v {
		return yes
	}
	return no
}

func checkTargeting(user UserInput, t Targeting) (bool, string, string) {
	if t.RequireRegistered && !user.Registered {
		return false, "NOT_REGISTERED", "experiment requires a registered account"
	}
	if user.AccountAgeDays < t.MinAccountAgeDays {
		return false, "ACCOUNT_TOO_NEW", fmt.Sprintf("account age %d days is below required %d days", user.AccountAgeDays, t.MinAccountAgeDays)
	}
	if len(t.Countries) > 0 {
		allowed := false
		upper := strings.ToUpper(user.Country)
		for _, c := range t.Countries {
			if strings.ToUpper(c) == upper {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, "COUNTRY_NOT_ALLOWED", fmt.Sprintf("country %q is not in allowed list %v", user.Country, t.Countries)
		}
	}
	return true, "", "all targeting rules passed"
}

func formatTargeting(t Targeting) string {
	parts := []string{}
	if len(t.Countries) > 0 {
		parts = append(parts, fmt.Sprintf("country in %v", t.Countries))
	}
	if t.RequireRegistered {
		parts = append(parts, "registered=true")
	}
	if t.MinAccountAgeDays > 0 {
		parts = append(parts, fmt.Sprintf("account_age_days>=%d", t.MinAccountAgeDays))
	}
	if len(parts) == 0 {
		return "No targeting restrictions"
	}
	return strings.Join(parts, "; ")
}
