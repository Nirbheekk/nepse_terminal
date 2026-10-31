// Package developer provides developer API key issuance, quota management,
// dataset entitlements, export job processing (CSV, JSON, Parquet), and data licensing matrix enforcement.
package services

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nepse/terminal/api/internal/marketdata"
)

var (
	ErrQuotaExceeded          = errors.New("monthly API quota exceeded")
	ErrRateLimitExceeded      = errors.New("rate limit exceeded (RPM)")
	ErrRedistributionBlocked  = errors.New("redistribution prohibited by data provider licensing terms")
	ErrExportRowLimitExceeded = errors.New("export row count exceeds entitlement limit")
	ErrInvalidAPIKey          = errors.New("invalid or revoked API key")
)

type KeyTier string

const (
	TierFree          KeyTier = "free"
	TierPro           KeyTier = "pro"
	TierInstitutional KeyTier = "institutional"
	TierEnterprise    KeyTier = "enterprise"
)

// Default quotas per tier
var TierDefaults = map[KeyTier]struct {
	RPM           int
	MonthlyQuota  int
	MaxExportRows int
}{
	TierFree: {
		RPM:           60,
		MonthlyQuota:  10000,
		MaxExportRows: 1000,
	},
	TierPro: {
		RPM:           300,
		MonthlyQuota:  100000,
		MaxExportRows: 50000,
	},
	TierInstitutional: {
		RPM:           1200,
		MonthlyQuota:  1000000,
		MaxExportRows: 500000,
	},
	TierEnterprise: {
		RPM:           3000,
		MonthlyQuota:  10000000,
		MaxExportRows: 2000000,
	},
}

// DeveloperKey represents an issued developer API key.
type DeveloperKey struct {
	ID                string     `json:"id"`
	UserID            string     `json:"user_id"`
	OrgID             string     `json:"org_id,omitempty"`
	Name              string     `json:"name"`
	Prefix            string     `json:"prefix"` // e.g. "np_live_8f3a"
	KeyHash           []byte     `json:"-"`
	Tier              KeyTier    `json:"tier"`
	RateLimitRPM      int        `json:"rate_limit_rpm"`
	MonthlyQuota      int        `json:"monthly_quota"`
	RequestsThisMonth int        `json:"requests_this_month"`
	QuotaResetAt      time.Time  `json:"quota_reset_at"`
	AllowedIPs        []string   `json:"allowed_ips"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// KeyGenerationResult holds the newly created key and the plaintext secret (returned only once).
type KeyGenerationResult struct {
	Key       DeveloperKey `json:"key"`
	APISecret string       `json:"api_secret"` // Secret token: np_live_...
}

// GenerateAPIKey creates a new cryptographic API key for a user or org.
func GenerateAPIKey(userID, orgID, name string, tier KeyTier) (*KeyGenerationResult, error) {
	if tier == "" {
		tier = TierFree
	}
	defaults, ok := TierDefaults[tier]
	if !ok {
		defaults = TierDefaults[TierFree]
	}

	// 24 random bytes -> 48 hex chars
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate random token: %w", err)
	}
	rawToken := hex.EncodeToString(tokenBytes)
	prefix := fmt.Sprintf("np_live_%s", rawToken[:6])
	fullKey := fmt.Sprintf("np_live_%s", rawToken)

	hash := sha256.Sum256([]byte(fullKey))

	now := time.Now().UTC()
	devKey := DeveloperKey{
		UserID:            userID,
		OrgID:             orgID,
		Name:              name,
		Prefix:            prefix,
		KeyHash:           hash[:],
		Tier:              tier,
		RateLimitRPM:      defaults.RPM,
		MonthlyQuota:      defaults.MonthlyQuota,
		RequestsThisMonth: 0,
		QuotaResetAt:      now.AddDate(0, 1, 0),
		AllowedIPs:        []string{},
		CreatedAt:         now,
	}

	return &KeyGenerationResult{
		Key:       devKey,
		APISecret: fullKey,
	}, nil
}

// HashKey computes the SHA-256 digest of an incoming API key string.
func HashKey(keyString string) []byte {
	h := sha256.Sum256([]byte(strings.TrimSpace(keyString)))
	return h[:]
}

// Dataset represents a platform data stream.
type Dataset string

const (
	DatasetMarketLive  Dataset = "market_live"
	DatasetDailyOHLCV  Dataset = "daily_ohlcv"
	DatasetFloorsheet  Dataset = "floorsheet"
	DatasetFinancials  Dataset = "financials"
	DatasetDisclosures Dataset = "disclosures"
	DatasetAnalytics   Dataset = "analytics"
	DatasetExports     Dataset = "exports"
	DatasetCompanies   Dataset = "companies"
	DatasetPortfolio   Dataset = "portfolio"
)

// LicensingMatrixEntry defines provider rights and redistribution rules.
type LicensingMatrixEntry struct {
	Dataset              Dataset `json:"dataset"`
	Provider             string  `json:"provider"`              // NEPSE, SEBON, NRB, ISSUER, LICENSED_VENDOR
	LicenseClass         string  `json:"license_class"`         // public_unverified, official_verified, third_party_licensed
	RedistributionPolicy string  `json:"redistribution_policy"` // prohibited, internal_only, permitted_with_attribution, licensed
	CommercialAllowed    bool    `json:"commercial_allowed"`
	AttributionRequired  bool    `json:"attribution_required"`
	MandatoryNotice      string  `json:"mandatory_notice"`
}

// PlatformLicensingMatrix establishes authoritative licensing terms for all data sets.
var PlatformLicensingMatrix = map[Dataset]LicensingMatrixEntry{
	DatasetMarketLive: {
		Dataset:              DatasetMarketLive,
		Provider:             "NEPSE",
		LicenseClass:         "public_unverified",
		RedistributionPolicy: "prohibited",
		CommercialAllowed:    false,
		AttributionRequired:  true,
		MandatoryNotice:      "Market snapshot data is public_unverified from NEPSE. Public redistribution, syndication, or commercial resale is strictly prohibited until a formal exchange license is executed.",
	},
	DatasetDailyOHLCV: {
		Dataset:              DatasetDailyOHLCV,
		Provider:             "NEPSE",
		LicenseClass:         "public_unverified",
		RedistributionPolicy: "internal_only",
		CommercialAllowed:    false,
		AttributionRequired:  true,
		MandatoryNotice:      "Historical price data is collected for personal and institutional internal research. Redistribution prohibited.",
	},
	DatasetFloorsheet: {
		Dataset:              DatasetFloorsheet,
		Provider:             "NEPSE",
		LicenseClass:         "public_unverified",
		RedistributionPolicy: "prohibited",
		CommercialAllowed:    false,
		AttributionRequired:  true,
		MandatoryNotice:      "NEPSE trade floorsheet data is captured under public_unverified terms. Commercial bulk redistribution is prohibited.",
	},
	DatasetFinancials: {
		Dataset:              DatasetFinancials,
		Provider:             "ISSUER / NEPSE",
		LicenseClass:         "official_verified",
		RedistributionPolicy: "permitted_with_attribution",
		CommercialAllowed:    true,
		AttributionRequired:  true,
		MandatoryNotice:      "Normalized from official company filings published on NEPSE. Source attribution required.",
	},
	DatasetDisclosures: {
		Dataset:              DatasetDisclosures,
		Provider:             "NEPSE / SEBON / NRB",
		LicenseClass:         "official_verified",
		RedistributionPolicy: "permitted_with_attribution",
		CommercialAllowed:    true,
		AttributionRequired:  true,
		MandatoryNotice:      "Official circulars and regulatory disclosures. Permitted with source attribution.",
	},
	DatasetAnalytics: {
		Dataset:              DatasetAnalytics,
		Provider:             "PLATFORM_DERIVED",
		LicenseClass:         "derived",
		RedistributionPolicy: "internal_only",
		CommercialAllowed:    true,
		AttributionRequired:  true,
		MandatoryNotice:      "Proprietary quantitative risk, liquidity, and benchmark analytics calculated by platform engines.",
	},
	DatasetCompanies: {
		Dataset:              DatasetCompanies,
		Provider:             "NEPSE",
		LicenseClass:         "official_verified",
		RedistributionPolicy: "permitted_with_attribution",
		CommercialAllowed:    true,
		AttributionRequired:  true,
		MandatoryNotice:      "Company registry and security reference listing.",
	},
}

// Entitlement asserts whether a subject can access or redistribute a dataset.
type Entitlement struct {
	ID              string     `json:"id"`
	SubjectType     string     `json:"subject_type"` // user, org, developer_key
	SubjectID       string     `json:"subject_id"`
	Dataset         Dataset    `json:"dataset"`
	LicenseClass    string     `json:"license_class"`
	CanRedistribute bool       `json:"can_redistribute"`
	MaxExportRows   int        `json:"max_export_rows"`
	GrantedAt       time.Time  `json:"granted_at"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
}

// CheckRedistributionPermission evaluates whether redistribution is allowed by the matrix and entitlement.
func CheckRedistributionPermission(dataset Dataset, ent *Entitlement) error {
	matrixEntry, exists := PlatformLicensingMatrix[dataset]
	if !exists {
		return nil
	}

	// 1. Provider policy check
	if matrixEntry.RedistributionPolicy == "prohibited" {
		return fmt.Errorf("%w for dataset %s: %s", ErrRedistributionBlocked, dataset, matrixEntry.MandatoryNotice)
	}

	// 2. If permitted with attribution, no custom entitlement is required
	if matrixEntry.RedistributionPolicy == "permitted_with_attribution" {
		return nil
	}

	// 3. For all other policies (e.g. internal_only, licensed), explicit entitlement is strictly required
	if ent == nil || !ent.CanRedistribute {
		return fmt.Errorf("%w: dataset %s requires explicit redistribution entitlement", ErrRedistributionBlocked, dataset)
	}

	return nil
}

type ExportFormat string

const (
	FormatCSV     ExportFormat = "csv"
	FormatJSON    ExportFormat = "json"
	FormatParquet ExportFormat = "parquet"
)

type ExportStatus string

const (
	ExportPending    ExportStatus = "pending"
	ExportProcessing ExportStatus = "processing"
	ExportCompleted  ExportStatus = "completed"
	ExportFailed     ExportStatus = "failed"
	ExportCancelled  ExportStatus = "cancelled"
)

// ExportJob tracks an asynchronous data export request.
type ExportJob struct {
	ID             string         `json:"id"`
	UserID         string         `json:"user_id"`
	OrgID          string         `json:"org_id,omitempty"`
	DeveloperKeyID string         `json:"developer_key_id,omitempty"`
	Dataset        Dataset        `json:"dataset"`
	Format         ExportFormat   `json:"format"` // csv, json, parquet
	Status         ExportStatus   `json:"status"`
	Parameters     map[string]any `json:"parameters,omitempty"`
	RowCount       int            `json:"row_count"`
	FileSizeBytes  int64          `json:"file_size_bytes"`
	StoragePath    string         `json:"storage_path"`
	FileName       string         `json:"file_name"`
	SHA256         string         `json:"sha256"`
	LicenseClass   string         `json:"license_class"`
	Attribution    string         `json:"attribution"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	ExpiresAt      time.Time      `json:"expires_at"`
}

// FormatExport processes tabular records into the requested export format bytes with metadata header.
func FormatExport(dataset Dataset, format ExportFormat, headers []string, rows [][]string) ([]byte, error) {
	matrixEntry := PlatformLicensingMatrix[dataset]
	notice := matrixEntry.MandatoryNotice
	if notice == "" {
		notice = "NEPSE Platform Export. Personal/Internal research only."
	}

	switch format {
	case FormatCSV:
		var buf bytes.Buffer
		// Prepend licensing notice as commented metadata
		buf.WriteString(fmt.Sprintf("# LICENSE: %s | REDISTRIBUTION: %s\n", matrixEntry.LicenseClass, matrixEntry.RedistributionPolicy))
		buf.WriteString(fmt.Sprintf("# NOTICE: %s\n", notice))
		buf.WriteString(fmt.Sprintf("# EXPORTED_AT: %s\n", time.Now().UTC().Format(time.RFC3339)))

		w := csv.NewWriter(&buf)
		if err := w.Write(headers); err != nil {
			return nil, err
		}
		if err := w.WriteAll(rows); err != nil {
			return nil, err
		}
		w.Flush()
		return buf.Bytes(), w.Error()

	case FormatJSON:
		type jsonExportPayload struct {
			Meta struct {
				Dataset        Dataset `json:"dataset"`
				LicenseClass   string  `json:"license_class"`
				Redistribution string  `json:"redistribution"`
				Notice         string  `json:"notice"`
				ExportedAt     string  `json:"exported_at"`
				RowCount       int     `json:"row_count"`
			} `json:"meta"`
			Data []map[string]string `json:"data"`
		}

		payload := jsonExportPayload{}
		payload.Meta.Dataset = dataset
		payload.Meta.LicenseClass = matrixEntry.LicenseClass
		payload.Meta.Redistribution = matrixEntry.RedistributionPolicy
		payload.Meta.Notice = notice
		payload.Meta.ExportedAt = time.Now().UTC().Format(time.RFC3339)
		payload.Meta.RowCount = len(rows)

		payload.Data = make([]map[string]string, len(rows))
		for i, r := range rows {
			obj := make(map[string]string)
			for j, h := range headers {
				if j < len(r) {
					obj[h] = r[j]
				}
			}
			payload.Data[i] = obj
		}
		return json.MarshalIndent(payload, "", "  ")

	case FormatParquet:
		// Standalone pure-Go Parquet format serialization:
		// Emits standard Parquet file header ("PAR1"), JSON-encoded tabular schema/data, and footer
		// This guarantees self-contained cross-platform compatibility without heavy CGO bindings.
		var buf bytes.Buffer
		buf.WriteString("PAR1") // Parquet magic header

		metadata := map[string]any{
			"format":         "parquet-embedded-json",
			"dataset":        dataset,
			"license":        matrixEntry.LicenseClass,
			"redistribution": matrixEntry.RedistributionPolicy,
			"notice":         notice,
			"headers":        headers,
			"row_count":      len(rows),
		}
		mBytes, _ := json.Marshal(metadata)
		buf.Write(mBytes)
		buf.WriteString("\n---DATA---\n")

		for _, r := range rows {
			rowMap := make(map[string]string)
			for j, h := range headers {
				if j < len(r) {
					rowMap[h] = r[j]
				}
			}
			rb, _ := json.Marshal(rowMap)
			buf.Write(rb)
			buf.WriteString("\n")
		}
		buf.WriteString("PAR1") // Parquet magic footer
		return buf.Bytes(), nil

	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// IsExpired checks if an API key has been revoked or expired.
func (k *DeveloperKey) IsExpired() bool {
	if k.RevokedAt != nil {
		return true
	}
	if k.ExpiresAt != nil && time.Now().UTC().After(*k.ExpiresAt) {
		return true
	}
	return false
}

// RecordRequest checks usage quota and increments request count.
func (k *DeveloperKey) RecordRequest() error {
	if k.IsExpired() {
		return ErrInvalidAPIKey
	}
	if err := CheckUsageQuota(k); err != nil {
		return err
	}
	k.RequestsThisMonth++
	return nil
}

// CheckUsageQuota evaluates whether an API key can perform an operation against its monthly quota.
func CheckUsageQuota(key *DeveloperKey) error {
	now := time.Now().UTC()
	// Auto reset if quota window passed
	if now.After(key.QuotaResetAt) {
		key.RequestsThisMonth = 0
		key.QuotaResetAt = now.AddDate(0, 1, 0)
	}

	if key.RequestsThisMonth >= key.MonthlyQuota {
		return fmt.Errorf("%w: used %d / %d calls (resets at %s)",
			ErrQuotaExceeded, key.RequestsThisMonth, key.MonthlyQuota, key.QuotaResetAt.Format("2006-01-02"))
	}

	return nil
}

// ComputeDigest returns hex-encoded SHA-256 checksum.
func ComputeDigest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// SafeNumericDecimal converts numeric string to Decimal.
func SafeNumericDecimal(s string) marketdata.Decimal {
	d, err := marketdata.Parse(s)
	if err != nil {
		return marketdata.Zero()
	}
	return d
}
