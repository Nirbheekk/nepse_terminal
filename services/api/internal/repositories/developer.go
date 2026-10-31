package repositories

import (
	"context"
	"encoding/json"
	"fmt"

	"nepse/terminal/api/internal/services"
)

// CreateDeveloperKey inserts a new developer API key.
func (s *Store) CreateDeveloperKey(ctx context.Context, k services.DeveloperKey) (string, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO developer_keys (
			user_id, org_id, name, prefix, key_hash, tier,
			rate_limit_rpm, monthly_quota, quota_reset_at, allowed_ips
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		k.UserID, nullableDate(k.OrgID), k.Name, k.Prefix, k.KeyHash,
		string(k.Tier), k.RateLimitRPM, k.MonthlyQuota, k.QuotaResetAt, k.AllowedIPs,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", fmt.Errorf("insert developer key: %w", err)
	}
	return id, nil
}

// GetDeveloperKeyByHash looks up an active developer key by its SHA-256 hash.
func (s *Store) GetDeveloperKeyByHash(ctx context.Context, hash []byte) (*services.DeveloperKey, error) {
	var k services.DeveloperKey
	var orgID *string
	var tStr string
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, org_id, name, prefix, key_hash, tier,
		       rate_limit_rpm, monthly_quota, requests_this_month, quota_reset_at,
		       allowed_ips, expires_at, revoked_at, last_used_at, created_at
		FROM developer_keys
		WHERE key_hash = $1 AND revoked_at IS NULL`, hash).Scan(
		&k.ID, &k.UserID, &orgID, &k.Name, &k.Prefix, &k.KeyHash, &tStr,
		&k.RateLimitRPM, &k.MonthlyQuota, &k.RequestsThisMonth, &k.QuotaResetAt,
		&k.AllowedIPs, &k.ExpiresAt, &k.RevokedAt, &k.LastUsedAt, &k.CreatedAt,
	)
	if err != nil {
		return nil, services.ErrInvalidAPIKey
	}
	k.Tier = services.KeyTier(tStr)
	if orgID != nil {
		k.OrgID = *orgID
	}
	return &k, nil
}

// ListDeveloperKeys returns all API keys for a user.
func (s *Store) ListDeveloperKeys(ctx context.Context, userID string) ([]services.DeveloperKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, org_id, name, prefix, tier,
		       rate_limit_rpm, monthly_quota, requests_this_month, quota_reset_at,
		       allowed_ips, expires_at, revoked_at, last_used_at, created_at
		FROM developer_keys
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []services.DeveloperKey
	for rows.Next() {
		var k services.DeveloperKey
		var orgID *string
		var tStr string
		if err := rows.Scan(
			&k.ID, &k.UserID, &orgID, &k.Name, &k.Prefix, &tStr,
			&k.RateLimitRPM, &k.MonthlyQuota, &k.RequestsThisMonth, &k.QuotaResetAt,
			&k.AllowedIPs, &k.ExpiresAt, &k.RevokedAt, &k.LastUsedAt, &k.CreatedAt,
		); err != nil {
			return nil, err
		}
		k.Tier = services.KeyTier(tStr)
		if orgID != nil {
			k.OrgID = *orgID
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeDeveloperKey revokes a developer API key.
func (s *Store) RevokeDeveloperKey(ctx context.Context, keyID, userID string) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE developer_keys
		SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, keyID, userID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("key not found or already revoked")
	}
	return nil
}

// IncrementDeveloperKeyUsage increments usage count and sets last_used_at.
func (s *Store) IncrementDeveloperKeyUsage(ctx context.Context, keyID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE developer_keys
		SET requests_this_month = requests_this_month + 1,
		    last_used_at = now()
		WHERE id = $1`, keyID)
	return err
}

// GrantEntitlement creates or updates dataset access rights.
func (s *Store) GrantEntitlement(ctx context.Context, ent services.Entitlement) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO entitlements (
			subject_type, subject_id, dataset, license_class, can_redistribute, max_export_rows, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (subject_type, subject_id, dataset) DO UPDATE SET
			license_class = EXCLUDED.license_class,
			can_redistribute = EXCLUDED.can_redistribute,
			max_export_rows = EXCLUDED.max_export_rows,
			expires_at = EXCLUDED.expires_at`,
		ent.SubjectType, ent.SubjectID, string(ent.Dataset), ent.LicenseClass,
		ent.CanRedistribute, ent.MaxExportRows, ent.ExpiresAt,
	)
	return err
}

// GetEntitlement retrieves dataset access rights.
func (s *Store) GetEntitlement(ctx context.Context, subjectType, subjectID string, dataset services.Dataset) (*services.Entitlement, error) {
	var ent services.Entitlement
	var dStr string
	err := s.pool.QueryRow(ctx, `
		SELECT id, subject_type, subject_id, dataset, license_class, can_redistribute, max_export_rows, granted_at, expires_at
		FROM entitlements
		WHERE subject_type = $1 AND subject_id = $2 AND dataset = $3`,
		subjectType, subjectID, string(dataset)).Scan(
		&ent.ID, &ent.SubjectType, &ent.SubjectID, &dStr, &ent.LicenseClass,
		&ent.CanRedistribute, &ent.MaxExportRows, &ent.GrantedAt, &ent.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	ent.Dataset = services.Dataset(dStr)
	return &ent, nil
}

// CreateExportJob registers an export request.
func (s *Store) CreateExportJob(ctx context.Context, job services.ExportJob) (string, error) {
	pBytes, _ := json.Marshal(job.Parameters)
	row := s.pool.QueryRow(ctx, `
		INSERT INTO export_jobs (
			user_id, org_id, developer_key_id, dataset, format, status,
			parameters, file_name, license_class, attribution
		) VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, $9)
		RETURNING id`,
		job.UserID, nullableDate(job.OrgID), nullableDate(job.DeveloperKeyID),
		string(job.Dataset), string(job.Format), pBytes, job.FileName,
		job.LicenseClass, job.Attribution,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// GetExportJob retrieves export job details.
func (s *Store) GetExportJob(ctx context.Context, jobID, userID string) (*services.ExportJob, error) {
	var job services.ExportJob
	var orgID, devKeyID *string
	var dStr, fStr, sStr string
	var pBytes []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, org_id, developer_key_id, dataset, format, status,
		       parameters, row_count, file_size_bytes, storage_path, file_name,
		       sha256, license_class, attribution, error_message, created_at,
		       completed_at, expires_at
		FROM export_jobs
		WHERE id = $1 AND user_id = $2`, jobID, userID).Scan(
		&job.ID, &job.UserID, &orgID, &devKeyID, &dStr, &fStr, &sStr,
		&pBytes, &job.RowCount, &job.FileSizeBytes, &job.StoragePath, &job.FileName,
		&job.SHA256, &job.LicenseClass, &job.Attribution, &job.ErrorMessage,
		&job.CreatedAt, &job.CompletedAt, &job.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	job.Dataset = services.Dataset(dStr)
	job.Format = services.ExportFormat(fStr)
	job.Status = services.ExportStatus(sStr)
	if orgID != nil {
		job.OrgID = *orgID
	}
	if devKeyID != nil {
		job.DeveloperKeyID = *devKeyID
	}
	_ = json.Unmarshal(pBytes, &job.Parameters)
	return &job, nil
}

// ListExportJobs lists export jobs for a user.
func (s *Store) ListExportJobs(ctx context.Context, userID string, limit, offset int) ([]services.ExportJob, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, org_id, developer_key_id, dataset, format, status,
		       parameters, row_count, file_size_bytes, storage_path, file_name,
		       sha256, license_class, attribution, error_message, created_at,
		       completed_at, expires_at
		FROM export_jobs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []services.ExportJob
	for rows.Next() {
		var j services.ExportJob
		var orgID, devKeyID *string
		var dStr, fStr, sStr string
		var pBytes []byte
		if err := rows.Scan(
			&j.ID, &j.UserID, &orgID, &devKeyID, &dStr, &fStr, &sStr,
			&pBytes, &j.RowCount, &j.FileSizeBytes, &j.StoragePath, &j.FileName,
			&j.SHA256, &j.LicenseClass, &j.Attribution, &j.ErrorMessage,
			&j.CreatedAt, &j.CompletedAt, &j.ExpiresAt,
		); err != nil {
			return nil, err
		}
		j.Dataset = services.Dataset(dStr)
		j.Format = services.ExportFormat(fStr)
		j.Status = services.ExportStatus(sStr)
		if orgID != nil {
			j.OrgID = *orgID
		}
		if devKeyID != nil {
			j.DeveloperKeyID = *devKeyID
		}
		_ = json.Unmarshal(pBytes, &j.Parameters)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// CompleteExportJob marks an export job as successfully generated.
func (s *Store) CompleteExportJob(ctx context.Context, jobID string, rowCount int, sizeBytes int64, path, sha string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE export_jobs
		SET status = 'completed',
		    row_count = $1,
		    file_size_bytes = $2,
		    storage_path = $3,
		    sha256 = $4,
		    completed_at = now()
		WHERE id = $1`, rowCount, sizeBytes, path, sha, jobID)
	return err
}

// FailExportJob records a failed export job.
func (s *Store) FailExportJob(ctx context.Context, jobID, msg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE export_jobs
		SET status = 'failed',
		    error_message = $1,
		    completed_at = now()
		WHERE id = $2`, msg, jobID)
	return err
}
