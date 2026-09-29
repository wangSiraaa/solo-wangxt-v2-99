package db

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/example/explainab/internal/experiment"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed schema.sql
var schemaSQL string

type APIError struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e APIError) Error() string { return e.Message }

type Repo struct{ DB *sql.DB }

func Open(ctx context.Context, url string) (*Repo, error) {
	database, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return &Repo{DB: database}, nil
}

func (r *Repo) Migrate(ctx context.Context) error {
	_, err := r.DB.ExecContext(ctx, schemaSQL)
	return err
}

func (r *Repo) Seed(ctx context.Context, layers []experiment.Layer) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM layers`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return tx.Commit()
	}
	for _, layer := range layers {
		var layerID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO layers(key, name, hash_namespace, salt)
			VALUES ($1,$2,$3,$4) RETURNING id`,
			layer.Key, layer.Name, layer.HashNamespace, layer.Salt).Scan(&layerID)
		if err != nil {
			return mapDBError(err)
		}
		for _, exp := range layer.Experiments {
			raw, err := json.Marshal(exp.Targeting)
			if err != nil {
				return err
			}
			var expID int64
			err = tx.QueryRowContext(ctx, `
				INSERT INTO experiments(layer_id, key, name, bucket_start, bucket_end, targeting, active)
				VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
				layerID, exp.Key, exp.Name, exp.BucketStart, exp.BucketEnd, raw, exp.Active).Scan(&expID)
			if err != nil {
				return mapDBError(err)
			}
			for _, v := range exp.Variants {
				_, err := tx.ExecContext(ctx, `
					INSERT INTO variants(experiment_id, key, name, weight)
					VALUES ($1,$2,$3,$4)`, expID, v.Key, v.Name, v.Weight)
				if err != nil {
					return mapDBError(err)
				}
			}
			for _, wl := range exp.Whitelist {
				_, err := tx.ExecContext(ctx, `
					INSERT INTO whitelist(layer_id, experiment_id, user_id, variant_key, reason)
					VALUES ($1,$2,$3,$4,$5)`,
					layerID, expID, wl.UserID, wl.VariantKey, wl.Reason)
				if err != nil {
					return mapDBError(err)
				}
			}
		}
	}
	return tx.Commit()
}

func (r *Repo) ListLayers(ctx context.Context) ([]experiment.Layer, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, key, name, hash_namespace, salt
		FROM layers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var layers []experiment.Layer
	for rows.Next() {
		var l experiment.Layer
		if err := rows.Scan(&l.ID, &l.Key, &l.Name, &l.HashNamespace, &l.Salt); err != nil {
			return nil, err
		}
		layers = append(layers, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range layers {
		exps, err := r.listExperiments(ctx, layers[i].ID)
		if err != nil {
			return nil, err
		}
		layers[i].Experiments = exps
	}
	return layers, nil
}

func (r *Repo) GetLayerByKey(ctx context.Context, key string) (experiment.Layer, error) {
	layers, err := r.ListLayers(ctx)
	if err != nil {
		return experiment.Layer{}, err
	}
	for _, layer := range layers {
		if layer.Key == key {
			return layer, nil
		}
	}
	return experiment.Layer{}, APIError{Status: http.StatusNotFound, Code: "LAYER_NOT_FOUND", Message: "layer not found"}
}

func (r *Repo) listExperiments(ctx context.Context, layerID int64) ([]experiment.Experiment, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, layer_id, key, name, bucket_start, bucket_end, targeting, active
		FROM experiments WHERE layer_id=$1 ORDER BY bucket_start`, layerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var exps []experiment.Experiment
	for rows.Next() {
		var exp experiment.Experiment
		var raw []byte
		if err := rows.Scan(&exp.ID, &exp.LayerID, &exp.Key, &exp.Name, &exp.BucketStart, &exp.BucketEnd, &raw, &exp.Active); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &exp.Targeting); err != nil {
			return nil, err
		}
		exps = append(exps, exp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range exps {
		variants, err := r.listVariants(ctx, exps[i].ID)
		if err != nil {
			return nil, err
		}
		exps[i].Variants = variants
		whitelists, err := r.listWhitelist(ctx, exps[i].ID)
		if err != nil {
			return nil, err
		}
		exps[i].Whitelist = whitelists
	}
	return exps, nil
}

func (r *Repo) listVariants(ctx context.Context, expID int64) ([]experiment.Variant, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, experiment_id, key, name, weight
		FROM variants WHERE experiment_id=$1 ORDER BY key`, expID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []experiment.Variant
	for rows.Next() {
		var v experiment.Variant
		if err := rows.Scan(&v.ID, &v.ExperimentID, &v.Key, &v.Name, &v.Weight); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func (r *Repo) listWhitelist(ctx context.Context, expID int64) ([]experiment.Whitelist, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, layer_id, experiment_id, user_id, variant_key, reason
		FROM whitelist WHERE experiment_id=$1 ORDER BY user_id`, expID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []experiment.Whitelist
	for rows.Next() {
		var w experiment.Whitelist
		if err := rows.Scan(&w.ID, &w.LayerID, &w.ExperimentID, &w.UserID, &w.VariantKey, &w.Reason); err != nil {
			return nil, err
		}
		values = append(values, w)
	}
	return values, rows.Err()
}

type PersistDecision struct {
	experiment.Decision
	LayerID      int64
	ExperimentID int64
}

func (r *Repo) PersistAssignmentAndExposure(ctx context.Context, user experiment.UserInput, layer experiment.Layer, d experiment.Decision) (experiment.Decision, error) {
	if !user.Persist || !d.Assigned {
		return d, nil
	}
	selected := findExperiment(layer, d.ExperimentKey)
	if selected == nil {
		return d, APIError{Status: http.StatusConflict, Code: "EXPERIMENT_NOT_IN_LAYER", Message: "decision experiment cannot be found"}
	}
	bucket := -1
	if d.Bucket != nil {
		bucket = *d.Bucket
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return d, err
	}
	defer func() { _ = tx.Rollback() }()

	var assignmentID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO assignments(layer_id, experiment_id, user_id, bucket, variant_key, source, reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (layer_id, user_id) DO NOTHING
		RETURNING id`,
		layer.ID, selected.ID, user.UserID, bucket, d.VariantKey, d.Source, d.Reason).Scan(&assignmentID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingExperimentID int64
		var existingVariant string
		err = tx.QueryRowContext(ctx, `
			SELECT id, experiment_id, variant_key
			FROM assignments
			WHERE layer_id=$1 AND user_id=$2`, layer.ID, user.UserID).
			Scan(&assignmentID, &existingExperimentID, &existingVariant)
		if err != nil {
			return d, mapDBError(err)
		}
		if existingExperimentID != selected.ID || existingVariant != d.VariantKey {
			return d, APIError{Status: http.StatusConflict, Code: "STICKY_ASSIGNMENT_MISMATCH", Message: "a sticky assignment already exists and the current configuration would produce a different group"}
		}
	} else if err != nil {
		return d, mapDBError(err)
	}
	if user.RecordExposure {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO exposures(layer_id, experiment_id, assignment_id, user_id, bucket, variant_key, source, reason)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			layer.ID, selected.ID, assignmentID, user.UserID, nullableBucket(bucket), d.VariantKey, d.Source, d.Reason)
		if err != nil {
			return d, mapDBError(err)
		}
		d.ExposureSaved = true
	}
	return d, tx.Commit()
}

func nullableBucket(bucket int) any {
	if bucket < 0 {
		return nil
	}
	return bucket
}

func findExperiment(layer experiment.Layer, key string) *experiment.Experiment {
	for i := range layer.Experiments {
		if layer.Experiments[i].Key == key {
			return &layer.Experiments[i]
		}
	}
	return nil
}

type StatRow struct {
	ExperimentKey  string `json:"experiment_key"`
	ExperimentName string `json:"experiment_name"`
	VariantKey     string `json:"variant_key"`
	Source         string `json:"source"`
	Reason         string `json:"reason,omitempty"`
	Assignments    int64  `json:"assignments"`
	Exposures      int64  `json:"exposures"`
}

func (r *Repo) Stats(ctx context.Context) ([]StatRow, error) {
	rows, err := r.DB.QueryContext(ctx, `
	WITH a AS (
		SELECT experiment_id, variant_key, source, reason, count(*) AS n
		FROM assignments GROUP BY 1,2,3,4
	), e AS (
		SELECT experiment_id, variant_key, source, reason, count(*) AS n
		FROM exposures GROUP BY 1,2,3,4
	), keys AS (
		SELECT experiment_id, variant_key, source, reason FROM a
		UNION
		SELECT experiment_id, variant_key, source, reason FROM e
	)
	SELECT x.key, x.name, k.variant_key, k.source, NULLIF(k.reason, ''), COALESCE(a.n,0), COALESCE(e.n,0)
	FROM keys k
	JOIN experiments x ON x.id=k.experiment_id
	LEFT JOIN a ON a.experiment_id=k.experiment_id AND a.variant_key=k.variant_key AND a.source=k.source AND a.reason=k.reason
	LEFT JOIN e ON e.experiment_id=k.experiment_id AND e.variant_key=k.variant_key AND e.source=k.source AND e.reason=k.reason
	ORDER BY x.key, k.source, k.variant_key, k.reason`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []StatRow
	for rows.Next() {
		var row StatRow
		var reason sql.NullString
		if err := rows.Scan(&row.ExperimentKey, &row.ExperimentName, &row.VariantKey, &row.Source, &reason, &row.Assignments, &row.Exposures); err != nil {
			return nil, err
		}
		row.Reason = reason.String
		values = append(values, row)
	}
	return values, rows.Err()
}

func (r *Repo) CreateLayer(ctx context.Context, layer experiment.Layer) (experiment.Layer, error) {
	if err := validateLayer(layer); err != nil {
		return experiment.Layer{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return experiment.Layer{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, `INSERT INTO layers(key,name,hash_namespace,salt) VALUES($1,$2,$3,$4) RETURNING id`,
		layer.Key, layer.Name, layer.HashNamespace, layer.Salt).Scan(&layer.ID); err != nil {
		return experiment.Layer{}, mapDBError(err)
	}
	for i := range layer.Experiments {
		if err := insertExperiment(ctx, tx, layer.ID, &layer.Experiments[i]); err != nil {
			return experiment.Layer{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return experiment.Layer{}, mapDBError(err)
	}
	return layer, nil
}

func insertExperiment(ctx context.Context, tx *sql.Tx, layerID int64, exp *experiment.Experiment) error {
	if err := validateExperiment(*exp); err != nil {
		return err
	}
	raw, err := json.Marshal(exp.Targeting)
	if err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO experiments(layer_id,key,name,bucket_start,bucket_end,targeting,active)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		layerID, exp.Key, exp.Name, exp.BucketStart, exp.BucketEnd, raw, exp.Active).Scan(&exp.ID); err != nil {
		return mapDBError(err)
	}
	exp.LayerID = layerID
	for i := range exp.Variants {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO variants(experiment_id,key,name,weight) VALUES($1,$2,$3,$4) RETURNING id`,
			exp.ID, exp.Variants[i].Key, exp.Variants[i].Name, exp.Variants[i].Weight).Scan(&exp.Variants[i].ID); err != nil {
			return mapDBError(err)
		}
		exp.Variants[i].ExperimentID = exp.ID
	}
	for i := range exp.Whitelist {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO whitelist(layer_id,experiment_id,user_id,variant_key,reason) VALUES($1,$2,$3,$4,$5) RETURNING id`,
			layerID, exp.ID, exp.Whitelist[i].UserID, exp.Whitelist[i].VariantKey, exp.Whitelist[i].Reason).Scan(&exp.Whitelist[i].ID); err != nil {
			return mapDBError(err)
		}
		exp.Whitelist[i].LayerID = layerID
		exp.Whitelist[i].ExperimentID = exp.ID
	}
	return nil
}

func validateLayer(layer experiment.Layer) error {
	if err := experiment.ValidateID(layer.Key, "layer key"); err != nil {
		return APIError{Status: http.StatusBadRequest, Code: "INVALID_LAYER_KEY", Message: err.Error()}
	}
	if strings.TrimSpace(layer.Name) == "" {
		return APIError{Status: http.StatusBadRequest, Code: "INVALID_NAME", Message: "layer name is required"}
	}
	if err := experiment.ValidateConfig([]experiment.Layer{layer}); err != nil {
		return APIError{Status: http.StatusBadRequest, Code: "INVALID_CONFIG", Message: err.Error()}
	}
	return nil
}

func validateExperiment(exp experiment.Experiment) error {
	if err := experiment.ValidateID(exp.Key, "experiment key"); err != nil {
		return APIError{Status: http.StatusBadRequest, Code: "INVALID_EXPERIMENT_KEY", Message: err.Error()}
	}
	if strings.TrimSpace(exp.Name) == "" {
		return APIError{Status: http.StatusBadRequest, Code: "INVALID_NAME", Message: "experiment name is required"}
	}
	return nil
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "SQLSTATE 23505"):
		return APIError{Status: http.StatusConflict, Code: "UNIQUE_CONSTRAINT", Message: "a record with the same key or mutually exclusive assignment already exists"}
	case strings.Contains(msg, "SQLSTATE 23P01"):
		return APIError{Status: http.StatusConflict, Code: "OVERLAPPING_RANGE", Message: "mutually exclusive experiment bucket intervals overlap"}
	default:
		return err
	}
}

var ErrNotFound = errors.New("not found")

func AsAPIError(err error) (APIError, bool) {
	var apiErr APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return APIError{Status: http.StatusInternalServerError, Code: "INTERNAL", Message: fmt.Sprintf("internal error: %v", err)}, false
}
