package repositories

import (
	"context"
	"fmt"
	"time"

	"nepse/terminal/api/internal/services"
)

// InsertDisclosure inserts an authorized disclosure and returns its generated UUID.
func (s *Store) InsertDisclosure(ctx context.Context, d services.Disclosure) (string, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO disclosures (
			authority, symbol, category, title, summary, body,
			published_at, effective_date, source_url, reference_number,
			license_class, redistribution, is_verified
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		) RETURNING id`,
		string(d.Authority), d.Symbol, string(d.Category), d.Title, d.Summary, d.Body,
		d.PublishedAt, nullableDate(d.EffectiveDate), d.SourceURL, d.ReferenceNumber,
		string(d.LicenseClass), string(d.Redistribution), d.IsVerified,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", fmt.Errorf("insert disclosure: %w", err)
	}

	// Insert attachments if any
	if len(d.Attachments) > 0 {
		for _, att := range d.Attachments {
			_, err := s.pool.Exec(ctx, `
				INSERT INTO disclosure_attachments (
					disclosure_id, file_name, file_url, file_size_bytes, mime_type, sha256, storage_key
				) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				id, att.FileName, att.FileURL, att.FileSizeBytes, att.MimeType, att.SHA256, att.StorageKey,
			)
			if err != nil {
				return id, fmt.Errorf("insert attachment %s: %w", att.FileName, err)
			}
		}
	}

	return id, nil
}

// ListDisclosures queries disclosures with optional filters.
func (s *Store) ListDisclosures(ctx context.Context, authority, symbol, category string, limit, offset int) ([]services.Disclosure, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, authority, symbol, category, title, summary, body,
		       published_at, effective_date, source_url, reference_number,
		       license_class, redistribution, is_verified, created_at
		FROM disclosures
		WHERE 1=1`
	args := []any{}
	argIdx := 1

	if authority != "" {
		query += fmt.Sprintf(" AND authority = $%d", argIdx)
		args = append(args, authority)
		argIdx++
	}
	if symbol != "" {
		query += fmt.Sprintf(" AND symbol = $%d", argIdx)
		args = append(args, symbol)
		argIdx++
	}
	if category != "" {
		query += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY published_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []services.Disclosure
	for rows.Next() {
		var d services.Disclosure
		var auth, cat, lic, red string
		var effDate *time.Time
		var pubAt, crAt time.Time

		if err := rows.Scan(
			&d.ID, &auth, &d.Symbol, &cat, &d.Title, &d.Summary, &d.Body,
			&pubAt, &effDate, &d.SourceURL, &d.ReferenceNumber,
			&lic, &red, &d.IsVerified, &crAt,
		); err != nil {
			return nil, err
		}

		d.Authority = services.Authority(auth)
		d.Category = services.Category(cat)
		d.LicenseClass = services.LicenseClass(lic)
		d.Redistribution = services.RedistributionRights(red)
		d.PublishedAt = pubAt
		d.CreatedAt = crAt
		npt := pubAt.In(services.NepalLocation)
		d.PublishedAtNPT = npt.Format("2006-01-02 15:04:05 MST")
		if effDate != nil {
			d.EffectiveDate = effDate.Format("2006-01-02")
		}

		// Load attachments
		attRows, err := s.pool.Query(ctx, `
			SELECT id, disclosure_id, file_name, file_url, file_size_bytes, mime_type, sha256, storage_key
			FROM disclosure_attachments
			WHERE disclosure_id = $1`, d.ID)
		if err == nil {
			for attRows.Next() {
				var att services.Attachment
				if err := attRows.Scan(
					&att.ID, &att.DisclosureID, &att.FileName, &att.FileURL,
					&att.FileSizeBytes, &att.MimeType, &att.SHA256, &att.StorageKey,
				); err == nil {
					d.Attachments = append(d.Attachments, att)
				}
			}
			attRows.Close()
		}

		result = append(result, d)
	}

	return result, rows.Err()
}

// GetDisclosure retrieves a single disclosure by ID.
func (s *Store) GetDisclosure(ctx context.Context, id string) (*services.Disclosure, error) {
	var d services.Disclosure
	var auth, cat, lic, red string
	var effDate *time.Time
	var pubAt, crAt time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT id, authority, symbol, category, title, summary, body,
		       published_at, effective_date, source_url, reference_number,
		       license_class, redistribution, is_verified, created_at
		FROM disclosures
		WHERE id = $1`, id).Scan(
		&d.ID, &auth, &d.Symbol, &cat, &d.Title, &d.Summary, &d.Body,
		&pubAt, &effDate, &d.SourceURL, &d.ReferenceNumber,
		&lic, &red, &d.IsVerified, &crAt,
	)
	if err != nil {
		return nil, err
	}

	d.Authority = services.Authority(auth)
	d.Category = services.Category(cat)
	d.LicenseClass = services.LicenseClass(lic)
	d.Redistribution = services.RedistributionRights(red)
	d.PublishedAt = pubAt
	d.CreatedAt = crAt
	npt := pubAt.In(services.NepalLocation)
	d.PublishedAtNPT = npt.Format("2006-01-02 15:04:05 MST")
	if effDate != nil {
		d.EffectiveDate = effDate.Format("2006-01-02")
	}

	// Load attachments
	attRows, err := s.pool.Query(ctx, `
		SELECT id, disclosure_id, file_name, file_url, file_size_bytes, mime_type, sha256, storage_key
		FROM disclosure_attachments
		WHERE disclosure_id = $1`, d.ID)
	if err == nil {
		for attRows.Next() {
			var att services.Attachment
			if err := attRows.Scan(
				&att.ID, &att.DisclosureID, &att.FileName, &att.FileURL,
				&att.FileSizeBytes, &att.MimeType, &att.SHA256, &att.StorageKey,
			); err == nil {
				d.Attachments = append(d.Attachments, att)
			}
		}
		attRows.Close()
	}

	return &d, nil
}
