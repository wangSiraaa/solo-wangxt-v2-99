package storage

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

func SeedConfig() Config {
	now := time.Now().UTC()
	return Config{
		Layers: []Layer{
			{
				ID:         "checkout_layer",
				Namespace:  "growth",
				Name:       "Checkout mutual-exclusion layer",
				Salt:       "layer-salt-v1",
				BucketSize: 10000,
				Active:     true,
				CreatedAt:  now,
			},
		},
		Experiments: []Experiment{
			{
				ID:        "one_click_promo",
				LayerID:   "checkout_layer",
				Name:      "One-click promo message",
				Start:     0,
				End:       5000,
				Salt:      "variant-salt-v1",
				Active:    true,
				CreatedAt: now,
				Criteria: EligibilityCriteria{
					RequireRegistered: true,
					AllowedCountries:  []string{"US"},
					AllowedPlans:      []string{"free", "paid"},
				},
			},
			{
				ID:        "rewards_panel",
				LayerID:   "checkout_layer",
				Name:      "Rewards information panel",
				Start:     5000,
				End:       10000,
				Salt:      "variant-salt-v1",
				Active:    true,
				CreatedAt: now,
				Criteria: EligibilityCriteria{
					RequireRegistered: true,
					AllowedCountries:  []string{"US"},
					AllowedPlans:      []string{"paid"},
				},
			},
		},
		Variants: []Variant{
			{ID: "one_click_control", ExperimentID: "one_click_promo", Name: "control", Start: 0, End: 5000, Active: true, CreatedAt: now},
			{ID: "one_click_treatment", ExperimentID: "one_click_promo", Name: "treatment", Start: 5000, End: 10000, Active: true, CreatedAt: now, Payload: map[string]any{"button_text": "Apply offer"}},
			{ID: "rewards_control", ExperimentID: "rewards_panel", Name: "control", Start: 0, End: 5000, Active: true, CreatedAt: now},
			{ID: "rewards_treatment", ExperimentID: "rewards_panel", Name: "treatment", Start: 5000, End: 10000, Active: true, CreatedAt: now, Payload: map[string]any{"panel": "rewards-expanded"}},
		},
		Whitelists: []Whitelist{
			{
				ID:           "wl-vip-001-checkout",
				UserID:       "vip-001",
				LayerID:      "checkout_layer",
				ExperimentID: "rewards_panel",
				VariantID:    "rewards_treatment",
				Reason:       "support override for synthetic VIP walkthrough",
				Active:       true,
				CreatedAt:    now,
			},
		},
	}
}

type MemoryStore struct {
	mu          sync.RWMutex
	cfg         Config
	assignments map[string]Assignment
	exposures   map[string]Exposure
}

func NewMemoryStore(cfg Config) *MemoryStore {
	return &MemoryStore{cfg: cfg, assignments: map[string]Assignment{}, exposures: map[string]Exposure{}}
}

func (s *MemoryStore) Config() (Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg, nil
}

func (s *MemoryStore) UpsertLayer(layer Layer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Layers {
		if s.cfg.Layers[i].ID == layer.ID {
			s.cfg.Layers[i] = layer
			return nil
		}
	}
	s.cfg.Layers = append(s.cfg.Layers, layer)
	return nil
}

func (s *MemoryStore) UpsertExperiment(exp Experiment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Experiments {
		if s.cfg.Experiments[i].ID == exp.ID {
			s.cfg.Experiments[i] = exp
			return nil
		}
	}
	s.cfg.Experiments = append(s.cfg.Experiments, exp)
	return nil
}

func (s *MemoryStore) UpsertVariant(variant Variant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Variants {
		if s.cfg.Variants[i].ID == variant.ID {
			s.cfg.Variants[i] = variant
			return nil
		}
	}
	s.cfg.Variants = append(s.cfg.Variants, variant)
	return nil
}

func (s *MemoryStore) UpsertWhitelist(wl Whitelist) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Whitelists {
		if s.cfg.Whitelists[i].UserID == wl.UserID && s.cfg.Whitelists[i].LayerID == wl.LayerID {
			wl.ID = s.cfg.Whitelists[i].ID
			s.cfg.Whitelists[i] = wl
			return nil
		}
	}
	s.cfg.Whitelists = append(s.cfg.Whitelists, wl)
	return nil
}

func (s *MemoryStore) UpsertAssignment(a Assignment) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := a.Source + "|" + a.UserID + "|" + a.LayerID
	if old, ok := s.assignments[key]; ok {
		a.ID = old.ID
		a.CreatedAt = old.CreatedAt
		a.UpdatedAt = time.Now().UTC()
		s.assignments[key] = a
		return a, nil
	}
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	a.CreatedAt, a.UpdatedAt = now, now
	s.assignments[key] = a
	return a, nil
}

func (s *MemoryStore) InsertExposure(e *Exposure) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// An assignment can have many events in production, but this demo records
	// one exposure per stable assignment so repeated clicks cannot inflate it.
	if existing, ok := s.exposures[e.AssignmentID]; ok {
		*e = existing
		return nil
	}
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	s.exposures[e.AssignmentID] = *e
	return nil
}

func (s *MemoryStore) Stats(source string) (StatCounters, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counters := StatCounters{
		Assignments: map[string]int{},
		Exposures:   map[string]int{},
		Rejections:  map[string]int{},
	}
	cfg := s.cfg
	for _, a := range s.assignments {
		if source != "" && a.Source != source {
			continue
		}
		counters.AssignmentTotal++
		if a.Status == StatusAssigned {
			name := variantName(cfg.Variants, a.VariantID)
			if a.OverrideReason != "" {
				counters.WhitelistCount++
				name = "whitelist:" + name
			}
			counters.Assignments[name]++
		} else {
			counters.Rejections[a.RejectReason]++
		}
	}
	for _, e := range s.exposures {
		if source != "" && e.Source != source {
			continue
		}
		counters.ExposureTotal++
		name := variantName(cfg.Variants, e.VariantID)
		if e.OverrideReason != "" {
			name = "whitelist:" + name
		}
		counters.Exposures[name]++
	}
	return counters, nil
}

func variantName(variants []Variant, id string) string {
	for _, v := range variants {
		if v.ID == id {
			return v.ExperimentID + "/" + v.Name
		}
	}
	return id
}
