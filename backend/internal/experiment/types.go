package experiment

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

const BucketModulo = 10000

type Targeting struct {
	Countries         []string `json:"countries"`
	RequireRegistered bool     `json:"require_registered"`
	MinAccountAgeDays int      `json:"min_account_age_days"`
}

type Layer struct {
	ID            int64        `json:"id"`
	Key           string       `json:"key"`
	Name          string       `json:"name"`
	HashNamespace string       `json:"hash_namespace"`
	Salt          string       `json:"salt"`
	Experiments   []Experiment `json:"experiments,omitempty"`
}

type Experiment struct {
	ID          int64       `json:"id"`
	LayerID     int64       `json:"layer_id"`
	Key         string      `json:"key"`
	Name        string      `json:"name"`
	BucketStart int         `json:"bucket_start"`
	BucketEnd   int         `json:"bucket_end"`
	Targeting   Targeting   `json:"targeting"`
	Active      bool        `json:"active"`
	Variants    []Variant   `json:"variants,omitempty"`
	Whitelist   []Whitelist `json:"whitelist,omitempty"`
}

type Variant struct {
	ID           int64  `json:"id,omitempty"`
	ExperimentID int64  `json:"experiment_id,omitempty"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Weight       int    `json:"weight"`
}

type Whitelist struct {
	ID           int64  `json:"id,omitempty"`
	LayerID      int64  `json:"layer_id,omitempty"`
	ExperimentID int64  `json:"experiment_id,omitempty"`
	UserID       string `json:"user_id"`
	VariantKey   string `json:"variant_key"`
	Reason       string `json:"reason"`
}

type UserInput struct {
	UserID         string `json:"user_id" binding:"required"`
	Known          bool   `json:"known"`
	Country        string `json:"country"`
	Registered     bool   `json:"registered"`
	AccountAgeDays int    `json:"account_age_days"`
	Persist        bool   `json:"persist"`
	RecordExposure bool   `json:"record_exposure"`
}

type Step struct {
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	Detail        string `json:"detail"`
	HashInput     string `json:"hash_input,omitempty"`
	Bucket        *int   `json:"bucket,omitempty"`
	RangeStart    *int   `json:"range_start,omitempty"`
	RangeEnd      *int   `json:"range_end,omitempty"`
	ExperimentKey string `json:"experiment_key,omitempty"`
	VariantKey    string `json:"variant_key,omitempty"`
	ReasonCode    string `json:"reason_code,omitempty"`
}

type VariantRange struct {
	Variant Variant `json:"variant"`
	Start   int     `json:"start"`
	End     int     `json:"end"`
}

type Decision struct {
	UserID         string         `json:"user_id"`
	LayerKey       string         `json:"layer_key"`
	Eligible       bool           `json:"eligible"`
	Allocated      bool           `json:"allocated"`
	Assigned       bool           `json:"assigned"`
	ExposureSaved  bool           `json:"exposure_saved"`
	Status         string         `json:"status"`
	ReasonCode     string         `json:"reason_code,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	HashInput      string         `json:"hash_input,omitempty"`
	Bucket         *int           `json:"bucket,omitempty"`
	ExperimentKey  string         `json:"experiment_key,omitempty"`
	ExperimentName string         `json:"experiment_name,omitempty"`
	VariantKey     string         `json:"variant_key,omitempty"`
	Source         string         `json:"source,omitempty"`
	VariantRanges  []VariantRange `json:"variant_ranges,omitempty"`
	Trace          []Step         `json:"trace"`
}

type EvaluationResponse struct {
	UserID    string     `json:"user_id"`
	Decisions []Decision `json:"decisions"`
}

func StableBucket(input string) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(input))
	return int(h.Sum64() % uint64(BucketModulo))
}

func LayerHashInput(namespace, salt, userID string) string {
	return fmt.Sprintf("%s:%s:%s", namespace, salt, userID)
}

func VariantHashInput(namespace, salt, experimentKey, userID string) string {
	return fmt.Sprintf("%s:%s:%s:variant:%s", namespace, salt, experimentKey, userID)
}

func ValidateID(value, kind string) error {
	if len(value) < 2 || len(value) > 64 {
		return fmt.Errorf("%s must be 2-64 characters", kind)
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return fmt.Errorf("%s may contain only letters, numbers, '_', '-' and '.'", kind)
	}
	return nil
}

func ValidateConfig(layers []Layer) error {
	for _, layer := range layers {
		if strings.TrimSpace(layer.HashNamespace) == "" {
			return fmt.Errorf("layer %s requires a hash namespace", layer.Key)
		}
		seenWL := map[string]string{}
		sort.Slice(layer.Experiments, func(i, j int) bool {
			return layer.Experiments[i].BucketStart < layer.Experiments[j].BucketStart
		})
		previousActiveEnd := -1
		previousActiveKey := ""
		for i := range layer.Experiments {
			exp := &layer.Experiments[i]
			if !exp.Active {
				continue
			}
			if exp.BucketStart < 0 || exp.BucketEnd >= BucketModulo || exp.BucketStart > exp.BucketEnd {
				return fmt.Errorf("experiment %s has an invalid bucket interval", exp.Key)
			}
			if previousActiveEnd >= 0 && exp.BucketStart <= previousActiveEnd {
				return fmt.Errorf("experiments %s and %s overlap in layer %s", previousActiveKey, exp.Key, layer.Key)
			}
			previousActiveEnd = exp.BucketEnd
			previousActiveKey = exp.Key
			sum := 0
			for _, v := range exp.Variants {
				if v.Weight < 0 || v.Weight > BucketModulo {
					return fmt.Errorf("variant %s/%s has invalid weight", exp.Key, v.Key)
				}
				sum += v.Weight
			}
			if len(exp.Variants) > 0 && sum != BucketModulo {
				return fmt.Errorf("variants for %s must total 10000, got %d", exp.Key, sum)
			}
			for _, wl := range exp.Whitelist {
				if findVariant(exp.Variants, wl.VariantKey) == nil {
					return fmt.Errorf("whitelist for %s points at missing variant %s", exp.Key, wl.VariantKey)
				}
				if previous, ok := seenWL[wl.UserID]; ok {
					return fmt.Errorf("user %s is whitelisted in mutually exclusive experiments %s and %s", wl.UserID, previous, exp.Key)
				}
				seenWL[wl.UserID] = exp.Key
			}
		}
	}
	return nil
}

func findVariant(variants []Variant, key string) *Variant {
	for i := range variants {
		if variants[i].Key == key {
			return &variants[i]
		}
	}
	return nil
}

func buildVariantRanges(variants []Variant) []VariantRange {
	copied := append([]Variant(nil), variants...)
	sort.Slice(copied, func(i, j int) bool { return copied[i].Key < copied[j].Key })
	ranges := make([]VariantRange, 0, len(copied))
	cursor := 0
	for _, v := range copied {
		if v.Weight == 0 {
			continue
		}
		ranges = append(ranges, VariantRange{Variant: v, Start: cursor, End: cursor + v.Weight - 1})
		cursor += v.Weight
	}
	return ranges
}

func chooseVariant(variants []Variant, bucket int) *Variant {
	for _, vr := range buildVariantRanges(variants) {
		if bucket >= vr.Start && bucket <= vr.End {
			v := vr.Variant
			return &v
		}
	}
	return nil
}
