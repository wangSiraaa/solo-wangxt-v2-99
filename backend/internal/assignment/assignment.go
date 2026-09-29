package assignment

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/example/ab-platform/internal/storage"
)

const (
	StatusAssigned = "assigned"
	StatusRejected = "rejected"

	ReasonUnknownIdentity = "UNKNOWN_IDENTITY"
	ReasonInactiveLayer   = "INACTIVE_LAYER"
	ReasonNoExperiment    = "NO_EXPERIMENT_FOR_BUCKET"
	ReasonInactive        = "INACTIVE_EXPERIMENT"
	ReasonNotRegistered   = "NOT_REGISTERED"
	ReasonCountryBlocked  = "COUNTRY_NOT_ALLOWED"
	ReasonPlanBlocked     = "PLAN_NOT_ALLOWED"
	ReasonWhitelistMiss   = "WHITELIST_TARGET_UNAVAILABLE"
	ReasonNoVariant       = "NO_VARIANT_FOR_BUCKET"
)

type Step struct {
	Name    string         `json:"name"`
	Passed  bool           `json:"passed"`
	Detail  string         `json:"detail"`
	Context map[string]any `json:"context,omitempty"`
}

type Decision struct {
	UserID           string              `json:"user_id"`
	Layer            *storage.Layer      `json:"layer,omitempty"`
	Bucket           int                 `json:"bucket"`
	HashInput        string              `json:"hash_input"`
	Experiment       *storage.Experiment `json:"experiment,omitempty"`
	Variant          *storage.Variant    `json:"variant,omitempty"`
	VariantHashInput string              `json:"variant_hash_input,omitempty"`
	VariantBucket    int                 `json:"variant_bucket,omitempty"`
	Status           string              `json:"status"`
	RejectReason     string              `json:"reject_reason,omitempty"`
	Whitelist        *storage.Whitelist  `json:"whitelist,omitempty"`
	Path             []string            `json:"path"`
	Steps            []Step              `json:"steps"`
}

func StableBucket(input string, size int) (uint32, int, error) {
	if size <= 0 {
		return 0, 0, errors.New("bucket size must be positive")
	}
	hash := fnv1a32(input)
	return hash, int(hash % uint32(size)), nil
}

func LayerHashInput(namespace, layerKey, layerSalt, userID string) string {
	return strings.Join([]string{namespace, layerKey, layerSalt, userID}, ":")
}

func VariantHashInput(namespace, layerKey, layerSalt, experimentKey, experimentSalt, userID string) string {
	return strings.Join([]string{namespace, layerKey, layerSalt, experimentKey, experimentSalt, userID}, ":")
}

func ValidateConfig(cfg storage.Config) error {
	layers := map[string]storage.Layer{}
	for _, layer := range cfg.Layers {
		layers[layer.ID] = layer
		if layer.BucketSize <= 0 {
			return fmt.Errorf("layer %s has invalid bucket size", layer.ID)
		}
	}
	byLayer := map[string][]storage.Experiment{}
	for _, exp := range cfg.Experiments {
		layer, ok := layers[exp.LayerID]
		if !ok {
			return fmt.Errorf("experiment %s references unknown layer %s", exp.ID, exp.LayerID)
		}
		if exp.Active && (exp.Start < 0 || exp.End > layer.BucketSize || exp.End <= exp.Start) {
			return fmt.Errorf("experiment %s has invalid bucket interval", exp.ID)
		}
		byLayer[exp.LayerID] = append(byLayer[exp.LayerID], exp)
	}
	for layerID, experiments := range byLayer {
		sort.Slice(experiments, func(i, j int) bool { return experiments[i].Start < experiments[j].Start })
		for i := 1; i < len(experiments); i++ {
			if experiments[i].Active && experiments[i-1].Active && experiments[i].Start < experiments[i-1].End {
				return fmt.Errorf("layer %s has overlapping experiments %s and %s", layerID, experiments[i-1].ID, experiments[i].ID)
			}
		}
	}
	experiments := map[string]storage.Experiment{}
	for _, exp := range cfg.Experiments {
		experiments[exp.ID] = exp
	}
	byExperiment := map[string][]storage.Variant{}
	for _, variant := range cfg.Variants {
		exp, ok := experiments[variant.ExperimentID]
		if !ok {
			return fmt.Errorf("variant %s references unknown experiment %s", variant.ID, variant.ExperimentID)
		}
		layer := layers[exp.LayerID]
		if variant.Active && (variant.Start < 0 || variant.End > layer.BucketSize || variant.End <= variant.Start) {
			return fmt.Errorf("variant %s has invalid bucket interval", variant.ID)
		}
		byExperiment[variant.ExperimentID] = append(byExperiment[variant.ExperimentID], variant)
	}
	for experimentID, variants := range byExperiment {
		sort.Slice(variants, func(i, j int) bool { return variants[i].Start < variants[j].Start })
		for i := 1; i < len(variants); i++ {
			if variants[i].Active && variants[i-1].Active && variants[i].Start < variants[i-1].End {
				return fmt.Errorf("experiment %s has overlapping variants", experimentID)
			}
		}
	}
	return nil
}

// Evaluate performs deterministic allocation without recording anything.
// The decision is independent of API call order and process memory: only
// configuration, explicit subject attributes, user id, and salts affect it.
func Evaluate(cfg storage.Config, subject storage.Subject, layerID string) (*Decision, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(subject.UserID) == "" {
		return nil, errors.New("user_id is required")
	}
	layer, ok := findLayer(cfg.Layers, layerID)
	if !ok {
		return nil, fmt.Errorf("layer %q not found", layerID)
	}

	d := &Decision{
		UserID: subject.UserID,
		Layer:  &layer,
		Status: StatusRejected,
		Path:   []string{},
	}
	addStep(d, "identity", subject.UserID != "" && subject.Registered,
		fmt.Sprintf("user_id=%q registered=%v", subject.UserID, subject.Registered),
		map[string]any{"user_id": subject.UserID, "registered": subject.Registered, "source": subject.Source})
	if !subject.Registered {
		reject(d, ReasonUnknownIdentity, "identity -> reject")
		return d, nil
	}

	hashInput := LayerHashInput(layer.Namespace, layer.ID, layer.Salt, subject.UserID)
	_, bucket, err := StableBucket(hashInput, layer.BucketSize)
	if err != nil {
		return nil, err
	}
	d.HashInput, d.Bucket = hashInput, bucket
	addStep(d, "mutual_exclusion_layer", layer.Active,
		fmt.Sprintf("hash input %q => bucket %d/%d", hashInput, bucket, layer.BucketSize),
		map[string]any{"namespace": layer.Namespace, "layer_salt": layer.Salt, "bucket_size": layer.BucketSize})
	if !layer.Active {
		reject(d, ReasonInactiveLayer, "mutual-exclusion layer inactive -> reject")
		return d, nil
	}

	if w := findWhitelist(cfg.Whitelists, subject.UserID, layer.ID); w != nil {
		return evaluateWhitelist(cfg, d, subject, *w)
	}

	experiments := experimentsInLayer(cfg.Experiments, layer.ID)
	exp, err := selectExperiment(experiments, bucket)
	if err != nil {
		addStep(d, "bucket_to_experiment", false, err.Error(), map[string]any{"bucket": bucket})
		reject(d, ReasonNoExperiment, "bucket has no configured experiment -> reject")
		return d, nil
	}
	d.Experiment = &exp
	d.Path = append(d.Path, "hash-bucket -> "+exp.ID)
	addStep(d, "bucket_to_experiment", true,
		fmt.Sprintf("bucket %d is in [%d,%d) => experiment %q", bucket, exp.Start, exp.End, exp.ID),
		map[string]any{"experiment_id": exp.ID, "range": []int{exp.Start, exp.End}})

	if !exp.Active {
		addStep(d, "eligibility", false, "experiment is inactive", nil)
		reject(d, ReasonInactive, "experiment inactive -> reject")
		return d, nil
	}
	if reason, ok := checkEligibility(exp.Criteria, subject); !ok {
		addStep(d, "eligibility", false, reason, map[string]any{"country": subject.Country, "plan": subject.Plan})
		reject(d, reason, "eligibility failed -> reject without exposure")
		return d, nil
	}
	addStep(d, "eligibility", true, "registered status, country, and plan satisfy experiment criteria",
		map[string]any{"country": subject.Country, "plan": subject.Plan})

	return chooseVariant(cfg, d, layer, exp, subject.UserID)
}

func evaluateWhitelist(cfg storage.Config, d *Decision, subject storage.Subject, w storage.Whitelist) (*Decision, error) {
	d.Whitelist = &w
	addStep(d, "whitelist_override", false,
		fmt.Sprintf("forced assignment to experiment %s / variant %s; reason=%s", w.ExperimentID, w.VariantID, w.Reason),
		map[string]any{"reason": w.Reason, "experiment_id": w.ExperimentID, "variant_id": w.VariantID})
	if !w.Active {
		reject(d, ReasonWhitelistMiss, "inactive whitelist -> reject")
		return d, nil
	}
	exp, ok := findExperiment(cfg.Experiments, w.ExperimentID)
	if !ok || !exp.Active || exp.LayerID != d.Layer.ID {
		reject(d, ReasonWhitelistMiss, "whitelist experiment is unavailable -> reject")
		return d, nil
	}
	variant, ok := findVariant(cfg.Variants, w.VariantID)
	if !ok || !variant.Active || variant.ExperimentID != exp.ID {
		reject(d, ReasonWhitelistMiss, "whitelist variant is unavailable -> reject")
		return d, nil
	}
	d.Experiment = &exp
	d.Variant = &variant
	d.VariantBucket = 0
	d.Status = StatusAssigned
	d.Path = append(d.Path, "whitelist -> "+exp.ID+" -> "+variant.ID)
	d.Steps[len(d.Steps)-1].Passed = true
	addStep(d, "eligibility", true, "skipped normal eligibility and criteria because an active operator override exists",
		map[string]any{"override": true})
	return d, nil
}

func chooseVariant(cfg storage.Config, d *Decision, layer storage.Layer, exp storage.Experiment, userID string) (*Decision, error) {
	input := VariantHashInput(layer.Namespace, layer.ID, layer.Salt, exp.ID, exp.Salt, userID)
	_, bucket, err := StableBucket(input, layer.BucketSize)
	if err != nil {
		return nil, err
	}
	d.VariantHashInput, d.VariantBucket = input, bucket
	variants := variantsForExperiment(cfg.Variants, exp.ID)
	variant, err := selectVariant(variants, bucket)
	if err != nil {
		addStep(d, "bucket_to_variant", false, err.Error(), map[string]any{"bucket": bucket, "hash_input": input})
		reject(d, ReasonNoVariant, "variant bucket is not configured -> reject")
		return d, nil
	}
	d.Variant = &variant
	d.Status = StatusAssigned
	d.Path = append(d.Path, "variant-hash -> "+variant.ID)
	addStep(d, "bucket_to_variant", true,
		fmt.Sprintf("hash input %q => bucket %d in [%d,%d) => variant %q", input, bucket, variant.Start, variant.End, variant.ID),
		map[string]any{"experiment_salt": exp.Salt, "range": []int{variant.Start, variant.End}})
	return d, nil
}

func checkEligibility(c storage.EligibilityCriteria, s storage.Subject) (string, bool) {
	if c.RequireRegistered && !s.Registered {
		return ReasonNotRegistered, false
	}
	if len(c.AllowedCountries) > 0 && !contains(c.AllowedCountries, s.Country) {
		return ReasonCountryBlocked, false
	}
	if len(c.AllowedPlans) > 0 && !contains(c.AllowedPlans, s.Plan) {
		return ReasonPlanBlocked, false
	}
	return "", true
}

func reject(d *Decision, reason, path string) {
	d.Status = StatusRejected
	d.RejectReason = reason
	d.Path = append(d.Path, path)
}

func addStep(d *Decision, name string, passed bool, detail string, context map[string]any) {
	d.Steps = append(d.Steps, Step{Name: name, Passed: passed, Detail: detail, Context: context})
}

func selectExperiment(experiments []storage.Experiment, bucket int) (storage.Experiment, error) {
	sorted := append([]storage.Experiment(nil), experiments...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	for _, exp := range sorted {
		if bucket >= exp.Start && bucket < exp.End {
			return exp, nil
		}
	}
	return storage.Experiment{}, errors.New(ReasonNoExperiment)
}

func selectVariant(variants []storage.Variant, bucket int) (storage.Variant, error) {
	sorted := append([]storage.Variant(nil), variants...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	for _, v := range sorted {
		if bucket >= v.Start && bucket < v.End {
			return v, nil
		}
	}
	return storage.Variant{}, errors.New(ReasonNoVariant)
}

func fnv1a32(input string) uint32 {
	const offset = uint32(2166136261)
	const prime = uint32(16777619)
	hash := offset
	for _, b := range []byte(input) {
		hash ^= uint32(b)
		hash *= prime
	}
	return hash
}

func findLayer(items []storage.Layer, id string) (storage.Layer, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return storage.Layer{}, false
}

func findExperiment(items []storage.Experiment, id string) (storage.Experiment, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return storage.Experiment{}, false
}

func findVariant(items []storage.Variant, id string) (storage.Variant, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return storage.Variant{}, false
}

func findWhitelist(items []storage.Whitelist, userID, layerID string) *storage.Whitelist {
	for i := range items {
		if items[i].UserID == userID && items[i].LayerID == layerID && items[i].Active {
			return &items[i]
		}
	}
	return nil
}

func experimentsInLayer(items []storage.Experiment, layerID string) []storage.Experiment {
	var result []storage.Experiment
	for _, item := range items {
		if item.LayerID == layerID {
			result = append(result, item)
		}
	}
	return result
}

func variantsForExperiment(items []storage.Variant, experimentID string) []storage.Variant {
	var result []storage.Variant
	for _, item := range items {
		if item.ExperimentID == experimentID && item.Active {
			result = append(result, item)
		}
	}
	return result
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}
