-- API keys for the api_v2 difftest suite: one active key whose multiplier
-- lifts the per-key throttles out of the way of a long suite, one inactive
-- and one expired.
DELETE FROM api_key_usage_logs WHERE api_key_id BETWEEN 990301 AND 990309;
DELETE FROM api_keys WHERE id BETWEEN 990301 AND 990309;
INSERT INTO api_keys (id, name, key, contact_email, active, billing_active, rate_limit_multiplier, requests_count, expires_at, created_at, updated_at) VALUES
 (990301, 'v2 suite', 'estevao_v2fixture00000000000000000000000000000000000000001', 'v2@example.com', TRUE, TRUE, 100000, 0, NULL, '2026-01-01', '2026-01-01'),
 (990302, 'v2 inactive', 'estevao_v2fixture00000000000000000000000000000000000000002', 'v2@example.com', FALSE, TRUE, 1, 0, NULL, '2026-01-01', '2026-01-01'),
 (990303, 'v2 expired', 'estevao_v2fixture00000000000000000000000000000000000000003', 'v2@example.com', TRUE, TRUE, 1, 0, '2026-01-01', '2026-01-01', '2026-01-01');
