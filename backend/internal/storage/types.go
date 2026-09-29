package storage

import "time"

const (
	StatusAssigned = "assigned"
	StatusRejected = "rejected"
)

type Layer struct {
	ID         string    `json:"id"`
	Namespace  string    `json:"namespace"`
	Name       string    `json:"name"`
	Salt       string    `json:"salt"`
	BucketSize int       `json:"bucket_size"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
}

type EligibilityCriteria struct {
	RequireRegistered bool     `json:"require_registered"`
	AllowedCountries  []string `json:"allowed_countries,omitempty"`
	AllowedPlans      []string `json:"allowed_plans,omitempty"`
}

type Experiment struct {
	ID        string              `json:"id"`
	LayerID   string              `json:"layer_id"`
	Name      string              `json:"name"`
	Start     int                 `json:"start_bucket"`
	End       int                 `json:"end_bucket"`
	Salt      string              `json:"salt"`
	Active    bool                `json:"active"`
	Criteria  EligibilityCriteria `json:"criteria"`
	CreatedAt time.Time           `json:"created_at,omitempty"`
}

type Variant struct {
	ID           string         `json:"id"`
	ExperimentID string         `json:"experiment_id"`
	Name         string         `json:"name"`
	Start        int            `json:"start_bucket"`
	End          int            `json:"end_bucket"`
	Payload      map[string]any `json:"payload,omitempty"`
	Active       bool           `json:"active"`
	CreatedAt    time.Time      `json:"created_at,omitempty"`
}

type Whitelist struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	LayerID      string    `json:"layer_id"`
	ExperimentID string    `json:"experiment_id"`
	VariantID    string    `json:"variant_id"`
	Reason       string    `json:"reason"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}

type Subject struct {
	UserID     string `json:"user_id"`
	Registered bool   `json:"registered"`
	Country    string `json:"country"`
	Plan       string `json:"plan"`
	Source     string `json:"source"`
}

type Assignment struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	LayerID        string    `json:"layer_id"`
	ExperimentID   string    `json:"experiment_id,omitempty"`
	VariantID      string    `json:"variant_id,omitempty"`
	Bucket         int       `json:"bucket"`
	VariantBucket  int       `json:"variant_bucket,omitempty"`
	Status         string    `json:"status"`
	RejectReason   string    `json:"reject_reason,omitempty"`
	OverrideReason string    `json:"override_reason,omitempty"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type Exposure struct {
	ID             string    `json:"id"`
	AssignmentID   string    `json:"assignment_id"`
	UserID         string    `json:"user_id"`
	ExperimentID   string    `json:"experiment_id"`
	VariantID      string    `json:"variant_id"`
	Source         string    `json:"source"`
	OverrideReason string    `json:"override_reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Config struct {
	Layers      []Layer      `json:"layers"`
	Experiments []Experiment `json:"experiments"`
	Variants    []Variant    `json:"variants"`
	Whitelists  []Whitelist  `json:"whitelists"`
}

type StatCounters struct {
	Assignments     map[string]int `json:"assignments"`
	Exposures       map[string]int `json:"exposures"`
	Rejections      map[string]int `json:"rejections"`
	WhitelistCount  int            `json:"whitelist_assignments"`
	AssignmentTotal int            `json:"assignment_total"`
	ExposureTotal   int            `json:"exposure_total"`
}

type Store interface {
	Config() (Config, error)
	UpsertLayer(Layer) error
	UpsertExperiment(Experiment) error
	UpsertVariant(Variant) error
	UpsertWhitelist(Whitelist) error
	UpsertAssignment(Assignment) (Assignment, error)
	InsertExposure(*Exposure) error
	Stats(source string) (StatCounters, error)
}
