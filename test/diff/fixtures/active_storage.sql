-- Blobs at fixed ids (so their signed ids are stable) for the
-- active_storage suite; the suite uploads their bytes to the fake S3.
DELETE FROM active_storage_attachments WHERE blob_id BETWEEN 900001 AND 900099;
DELETE FROM active_storage_blobs WHERE id BETWEEN 900001 AND 900099;
INSERT INTO active_storage_blobs (id, key, filename, content_type, metadata, service_name, byte_size, checksum, created_at) VALUES
 (900001, 'difftestavatarpng0000000001', 'avatar image.png', 'image/png', '{"identified":true}', 'railway_avatars', 26, 'fS4rVpY6oxjwCP2aKi9f1A==', '2026-01-01 00:00:00'),
 (900002, 'difftestvector00000000000002', 'x "1".svg', 'image/svg+xml', '{"identified":true}', 'railway_avatars', 26, 'fS4rVpY6oxjwCP2aKi9f1A==', '2026-01-01 00:00:00'),
 (900003, 'difftestnotes000000000000003', 'notes;é.txt', 'text/plain', '{}', 'railway_avatars', 26, 'fS4rVpY6oxjwCP2aKi9f1A==', '2026-01-01 00:00:00'),
 (900004, 'difftestlegacy00000000000004', 'legacy.jpg', 'image/jpeg', '{}', 'production', 26, 'fS4rVpY6oxjwCP2aKi9f1A==', '2026-01-01 00:00:00'),
 (900005, 'difftestmissing0000000000005', 'gone.png', 'image/png', '{}', 'railway_avatars', 26, 'fS4rVpY6oxjwCP2aKi9f1A==', '2026-01-01 00:00:00'),
 (900006, 'difftestnotype00000000000006', 'blob', NULL, '{}', 'railway_avatars', 26, 'fS4rVpY6oxjwCP2aKi9f1A==', '2026-01-01 00:00:00'),
 (900007, 'difftestlocal000000000000007', 'local.png', 'image/png', '{}', 'local', 26, 'fS4rVpY6oxjwCP2aKi9f1A==', '2026-01-01 00:00:00');
