INSERT INTO layers (id, namespace, name, salt, bucket_size, active)
VALUES ('checkout_layer', 'growth', 'Checkout mutual-exclusion layer', 'layer-salt-v1', 10000, TRUE)
ON CONFLICT (id) DO UPDATE SET
  namespace = EXCLUDED.namespace, name = EXCLUDED.name, salt = EXCLUDED.salt,
  bucket_size = EXCLUDED.bucket_size, active = EXCLUDED.active;

INSERT INTO experiments (id, layer_id, name, start_bucket, end_bucket, salt, active, criteria) VALUES
('one_click_promo', 'checkout_layer', 'One-click promo message', 0, 5000, 'variant-salt-v1', TRUE,
 '{"require_registered":true,"allowed_countries":["US"],"allowed_plans":["free","paid"]}'::jsonb),
('rewards_panel', 'checkout_layer', 'Rewards information panel', 5000, 10000, 'variant-salt-v1', TRUE,
 '{"require_registered":true,"allowed_countries":["US"],"allowed_plans":["paid"]}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  layer_id = EXCLUDED.layer_id, name = EXCLUDED.name,
  start_bucket = EXCLUDED.start_bucket, end_bucket = EXCLUDED.end_bucket,
  salt = EXCLUDED.salt, active = EXCLUDED.active, criteria = EXCLUDED.criteria;

INSERT INTO variants (id, experiment_id, name, start_bucket, end_bucket, active, payload) VALUES
('one_click_control', 'one_click_promo', 'control', 0, 5000, TRUE, '{}'::jsonb),
('one_click_treatment', 'one_click_promo', 'treatment', 5000, 10000, TRUE, '{"button_text":"Apply offer"}'::jsonb),
('rewards_control', 'rewards_panel', 'control', 0, 5000, TRUE, '{}'::jsonb),
('rewards_treatment', 'rewards_panel', 'treatment', 5000, 10000, TRUE, '{"panel":"rewards-expanded"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  experiment_id = EXCLUDED.experiment_id, name = EXCLUDED.name,
  start_bucket = EXCLUDED.start_bucket, end_bucket = EXCLUDED.end_bucket,
  active = EXCLUDED.active, payload = EXCLUDED.payload;

INSERT INTO whitelists (id, user_id, layer_id, experiment_id, variant_id, reason, active)
SELECT 'wl-vip-001-checkout', 'vip-001', 'checkout_layer', 'rewards_panel', 'rewards_treatment',
       'support override for synthetic VIP walkthrough', TRUE
WHERE NOT EXISTS (SELECT 1 FROM whitelists WHERE user_id = 'vip-001' AND layer_id = 'checkout_layer');
