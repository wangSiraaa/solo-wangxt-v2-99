package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	store := &PostgresStore{pool: pool}
	if err := store.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := store.Migrate(ctx, "migrations"); err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) Migrate(ctx context.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *PostgresStore) Config() (Config, error) {
	ctx := context.Background()
	cfg := Config{}

	rows, err := s.pool.Query(ctx, `SELECT id, namespace, name, salt, bucket_size, active, created_at FROM layers ORDER BY id`)
	if err != nil {
		return cfg, err
	}
	cfg.Layers, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Layer])
	if err != nil {
		return cfg, err
	}

	rows, err = s.pool.Query(ctx, `SELECT id, layer_id, name, start_bucket, end_bucket, salt, active, criteria, created_at FROM experiments ORDER BY layer_id, start_bucket`)
	if err != nil {
		return cfg, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			exp          Experiment
			criteriaJSON []byte
		)
		if err := rows.Scan(
			&exp.ID, &exp.LayerID, &exp.Name, &exp.Start, &exp.End,
			&exp.Salt, &exp.Active, &criteriaJSON, &exp.CreatedAt,
		); err != nil {
			return cfg, err
		}
		if err := json.Unmarshal(criteriaJSON, &exp.Criteria); err != nil {
			return cfg, err
		}
		cfg.Experiments = append(cfg.Experiments, exp)
	}
	if err := rows.Err(); err != nil {
		return cfg, err
	}

	rows, err = s.pool.Query(ctx, `SELECT id, experiment_id, name, start_bucket, end_bucket, payload, active, created_at FROM variants ORDER BY experiment_id, start_bucket`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var (
			v           Variant
			payloadJSON []byte
		)
		if err := rows.Scan(
			&v.ID, &v.ExperimentID, &v.Name, &v.Start, &v.End,
			&payloadJSON, &v.Active, &v.CreatedAt,
		); err != nil {
			return cfg, err
		}
		if err := json.Unmarshal(payloadJSON, &v.Payload); err != nil {
			return cfg, err
		}
		cfg.Variants = append(cfg.Variants, v)
	}
	if err := rows.Err(); err != nil {
		return cfg, err
	}

	rows, err = s.pool.Query(ctx, `SELECT id, user_id, layer_id, experiment_id, variant_id, reason, active, created_at FROM whitelists ORDER BY user_id, layer_id`)
	if err != nil {
		return cfg, err
	}
	cfg.Whitelists, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Whitelist])
	return cfg, err
}

func (s *PostgresStore) UpsertLayer(l Layer) error {
	_, err := s.pool.Exec(context.Background(), `
INSERT INTO layers (id, namespace, name, salt, bucket_size, active, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (id) DO UPDATE SET namespace=EXCLUDED.namespace, name=EXCLUDED.name,
 salt=EXCLUDED.salt, bucket_size=EXCLUDED.bucket_size, active=EXCLUDED.active`,
		l.ID, l.Namespace, l.Name, l.Salt, l.BucketSize, l.Active, time.Now().UTC())
	return err
}

func (s *PostgresStore) UpsertExperiment(e Experiment) error {
	criteria, err := json.Marshal(e.Criteria)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(), `
INSERT INTO experiments (id, layer_id, name, start_bucket, end_bucket, salt, active, criteria, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (id) DO UPDATE SET layer_id=EXCLUDED.layer_id, name=EXCLUDED.name,
 start_bucket=EXCLUDED.start_bucket, end_bucket=EXCLUDED.end_bucket, salt=EXCLUDED.salt,
 active=EXCLUDED.active, criteria=EXCLUDED.criteria`,
		e.ID, e.LayerID, e.Name, e.Start, e.End, e.Salt, e.Active, criteria, time.Now().UTC())
	return err
}

func (s *PostgresStore) UpsertVariant(v Variant) error {
	payload, err := json.Marshal(v.Payload)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(), `
INSERT INTO variants (id, experiment_id, name, start_bucket, end_bucket, payload, active, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (id) DO UPDATE SET experiment_id=EXCLUDED.experiment_id, name=EXCLUDED.name,
 start_bucket=EXCLUDED.start_bucket, end_bucket=EXCLUDED.end_bucket,
 payload=EXCLUDED.payload, active=EXCLUDED.active`,
		v.ID, v.ExperimentID, v.Name, v.Start, v.End, payload, v.Active, time.Now().UTC())
	return err
}

func (s *PostgresStore) UpsertWhitelist(w Whitelist) error {
	_, err := s.pool.Exec(context.Background(), `
INSERT INTO whitelists (id, user_id, layer_id, experiment_id, variant_id, reason, active, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (id) DO UPDATE SET user_id=EXCLUDED.user_id, layer_id=EXCLUDED.layer_id,
 experiment_id=EXCLUDED.experiment_id, variant_id=EXCLUDED.variant_id,
 reason=EXCLUDED.reason, active=EXCLUDED.active`,
		w.ID, w.UserID, w.LayerID, w.ExperimentID, w.VariantID, w.Reason, w.Active, time.Now().UTC())
	return err
}

func (s *PostgresStore) UpsertAssignment(a Assignment) (Assignment, error) {
	ctx := context.Background()
	row := s.pool.QueryRow(ctx, `
INSERT INTO assignments (
  user_id, layer_id, experiment_id, variant_id, bucket, variant_bucket,
  status, reject_reason, override_reason, source, created_at, updated_at
) VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,now(),now())
ON CONFLICT (user_id, layer_id, source) DO UPDATE SET
  experiment_id=EXCLUDED.experiment_id, variant_id=EXCLUDED.variant_id,
  bucket=EXCLUDED.bucket, variant_bucket=EXCLUDED.variant_bucket,
  status=EXCLUDED.status, reject_reason=EXCLUDED.reject_reason,
  override_reason=EXCLUDED.override_reason, updated_at=now()
RETURNING id, created_at, updated_at`,
		a.UserID, a.LayerID, a.ExperimentID, a.VariantID, a.Bucket, a.VariantBucket,
		a.Status, a.RejectReason, a.OverrideReason, a.Source)
	if err := row.Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return Assignment{}, err
	}
	return a, nil
}

func (s *PostgresStore) InsertExposure(e *Exposure) error {
	row := s.pool.QueryRow(context.Background(), `
INSERT INTO exposures (assignment_id, user_id, experiment_id, variant_id, source, override_reason, created_at)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),now())
ON CONFLICT (assignment_id) DO UPDATE SET updated_at = now()
RETURNING id, created_at`,
		e.AssignmentID, e.UserID, e.ExperimentID, e.VariantID, e.Source, e.OverrideReason)
	return row.Scan(&e.ID, &e.CreatedAt)
}

func (s *PostgresStore) Stats(source string) (StatCounters, error) {
	ctx := context.Background()
	counters := StatCounters{Assignments: map[string]int{}, Exposures: map[string]int{}, Rejections: map[string]int{}}
	args := []any{}
	where := ""
	if source != "" {
		where = " WHERE source = $1"
		args = append(args, source)
	}

	rows, err := s.pool.Query(ctx, `
SELECT status, COALESCE(e.id,''), COALESCE(v.name,''), COALESCE(a.override_reason,''),
       COALESCE(a.reject_reason,''), COUNT(*)
FROM assignments a
LEFT JOIN experiments e ON a.experiment_id = e.id
LEFT JOIN variants v ON a.variant_id = v.id`+where+`
GROUP BY status, e.id, v.name, a.override_reason, a.reject_reason`, args...)
	if err != nil {
		return counters, err
	}
	defer rows.Close()
	for rows.Next() {
		var status, experimentID, variantName, override, rejectReason string
		var count int
		if err := rows.Scan(&status, &experimentID, &variantName, &override, &rejectReason, &count); err != nil {
			return counters, err
		}
		counters.AssignmentTotal += count
		if status == StatusAssigned {
			key := experimentID + "/" + variantName
			if override != "" {
				key = "whitelist:" + key
				counters.WhitelistCount += count
			}
			counters.Assignments[key] += count
		} else {
			counters.Rejections[rejectReason] += count
		}
	}
	if err := rows.Err(); err != nil {
		return counters, err
	}

	exposureWhere := ""
	if source != "" {
		exposureWhere = " WHERE x.source = $1"
	}
	rows2, err := s.pool.Query(ctx, `
SELECT e.id, v.name, COALESCE(x.override_reason,''), COUNT(*)
FROM exposures x JOIN experiments e ON x.experiment_id = e.id
JOIN variants v ON x.variant_id = v.id`+exposureWhere+`
GROUP BY e.id, v.name, x.override_reason`, args...)
	if err != nil {
		return counters, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var experimentID, variantName, override string
		var count int
		if err := rows2.Scan(&experimentID, &variantName, &override, &count); err != nil {
			return counters, err
		}
		counters.ExposureTotal += count
		key := experimentID + "/" + variantName
		if override != "" {
			key = "whitelist:" + key
		}
		counters.Exposures[key] += count
	}
	return counters, rows2.Err()
}
