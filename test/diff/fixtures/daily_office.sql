-- Fixtures for the daily_office difftest suite, shared by the Rails oracle and
-- the Go server (same database). Idempotent: rows are keyed by provider_uid /
-- short_code and replaced on every run.
BEGIN;
DELETE FROM completions WHERE user_id IN (SELECT id FROM users WHERE provider_uid LIKE 'difftest-do-%');
DELETE FROM feature_flags WHERE user_id IN (SELECT id FROM users WHERE provider_uid LIKE 'difftest-do-%');
DELETE FROM shared_offices WHERE short_code LIKE 'DTDO%';
DELETE FROM user_onboardings WHERE user_id IN (SELECT id FROM users WHERE provider_uid LIKE 'difftest-do-%');
DELETE FROM users WHERE provider_uid LIKE 'difftest-do-%';

INSERT INTO users (provider_uid, email, name, preferences, premium_expires_at, current_streak, longest_streak, timezone, created_at, updated_at) VALUES
 ('difftest-do-free', 'do-free@example.com', 'Free', '{"prayer_book_code":"loc_2015","bible_version":"nvi","lords_prayer_style":"traditional"}', NULL, 3, 9, 'America/Sao_Paulo', now(), now()),
 ('difftest-do-premium', 'do-premium@example.com', 'Premium', '{"prayer_book_code":"loc_2019","bible_version":"nvi","preferred_audio_voice":"female_1"}', '2099-01-01', 12, 40, 'Brasilia', now(), now()),
 ('difftest-do-expired', 'do-expired@example.com', 'Expired', '{"prayer_book_code":"loc_1549","bible_version":"nvi"}', '2020-01-01', 0, 1, 'UTC', now(), now()),
 ('difftest-do-noonboard', 'do-noonboard@example.com', 'NoOnboard', '{}', NULL, 0, 0, 'America/Sao_Paulo', now(), now()),
 ('difftest-do-english', 'do-english@example.com', 'English', '{"prayer_book_code":"loc_1979_en","bible_version":"kjv","daily_office_rite":"1"}', NULL, 1, 1, 'Pacific Time (US & Canada)', now(), now());

INSERT INTO user_onboardings (user_id, prayer_book_id, bible_version_id, mode, onboarding_completed, completed_at, preferences, created_at, updated_at)
SELECT u.id, pb.id, bv.id, 'basic', TRUE, now(), '{}', now(), now()
FROM users u
JOIN prayer_books pb ON pb.code = u.preferences->>'prayer_book_code'
JOIN bible_versions bv ON bv.code = u.preferences->>'bible_version'
WHERE u.provider_uid IN ('difftest-do-free', 'difftest-do-premium', 'difftest-do-expired', 'difftest-do-english');

INSERT INTO completions (user_id, date_reference, office_type, prayer_book_id, created_at, updated_at)
SELECT u.id, d::date, o, 1, '2026-04-05 09:30:00', '2026-04-05 09:30:00'
FROM users u, (VALUES ('2026-04-05'), ('2026-09-16')) AS dd(d), (VALUES ('morning'), ('evening')) AS oo(o)
WHERE u.provider_uid IN ('difftest-do-free', 'difftest-do-premium');

INSERT INTO feature_flags (feature_key, target_type, user_id, enabled, created_at, updated_at)
SELECT 'background_music', 'user', u.id, TRUE, now(), now() FROM users u WHERE u.provider_uid = 'difftest-do-premium';
INSERT INTO feature_flags (feature_key, target_type, user_id, enabled, created_at, updated_at)
SELECT 'yearly_wrapped', 'user', u.id, TRUE, now(), now() FROM users u WHERE u.provider_uid IN ('difftest-do-premium', 'difftest-do-free');

INSERT INTO shared_offices (short_code, prayer_book_code, office_type, date, seed, preferences, expires_at, created_at, updated_at) VALUES
 ('DTDOACTIVE', 'loc_2015', 'evening', '2026-04-05', 424242, '{"bible_version":"arc","lords_prayer_version":"traditional"}', '2099-01-01', now(), now()),
 ('DTDOENGLISH', 'loc_1979_en', 'morning', '2026-12-24', 7, '{"daily_office_rite":"1"}', '2099-01-01', now(), now()),
 ('DTDOFAMILY', 'loc_2015', 'compline', '2026-05-24', 99, '{"family_rite":true}', '2099-01-01', now(), now()),
 ('DTDOEXPIRED', 'loc_2015', 'morning', '2026-01-01', 1, '{}', '2020-01-01', now(), now());
COMMIT;
