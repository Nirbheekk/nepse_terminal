package services

import (
	"strings"
	"testing"
	"time"
)

func TestAPIKeyGenerationAndHashing(t *testing.T) {
	res, err := GenerateAPIKey("user-1", "org-1", "Test Production Key", TierPro)
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	if !strings.HasPrefix(res.APISecret, "np_live_") {
		t.Errorf("expected np_live_ prefix in api secret, got %s", res.APISecret)
	}
	if res.Key.Tier != TierPro {
		t.Errorf("expected TierPro, got %s", res.Key.Tier)
	}
	if res.Key.RateLimitRPM != 300 {
		t.Errorf("expected 300 RPM for TierPro, got %d", res.Key.RateLimitRPM)
	}
	if res.Key.MonthlyQuota != 100000 {
		t.Errorf("expected 100k quota for TierPro, got %d", res.Key.MonthlyQuota)
	}

	// Verify hashing matches incoming secret
	computedHash := HashKey(res.APISecret)
	if string(computedHash) != string(res.Key.KeyHash) {
		t.Error("computed hash did not match key hash")
	}
}

func TestQuotaEnforcementAndReset(t *testing.T) {
	key := &DeveloperKey{
		Tier:              TierFree,
		MonthlyQuota:      100,
		RequestsThisMonth: 100,
		QuotaResetAt:      time.Now().UTC().Add(24 * time.Hour), // Tomorrow
	}

	err := CheckUsageQuota(key)
	if err == nil {
		t.Error("expected quota exceeded error, got nil")
	}

	// Test auto-reset when reset date has passed
	key.QuotaResetAt = time.Now().UTC().Add(-2 * time.Hour) // 2 hours ago
	err = CheckUsageQuota(key)
	if err != nil {
		t.Errorf("expected auto reset to allow request, got %v", err)
	}
	if key.RequestsThisMonth != 0 {
		t.Errorf("expected requests_this_month to reset to 0, got %d", key.RequestsThisMonth)
	}
}

func TestLicensingMatrixAndRedistributionRestrictions(t *testing.T) {
	// 1. NEPSE Market Live is strictly prohibited from redistribution
	err := CheckRedistributionPermission(DatasetMarketLive, nil)
	if err == nil {
		t.Error("expected redistribution error for DatasetMarketLive")
	}
	if !strings.Contains(err.Error(), "redistribution prohibited") {
		t.Errorf("expected ErrRedistributionBlocked, got: %v", err)
	}

	// 2. Disclosures are permitted with attribution
	err = CheckRedistributionPermission(DatasetDisclosures, nil)
	if err != nil {
		t.Errorf("disclosures should be permitted with attribution, got: %v", err)
	}

	// 3. Daily OHLCV is internal_only without explicit redistribution entitlement
	err = CheckRedistributionPermission(DatasetDailyOHLCV, nil)
	if err == nil {
		t.Error("expected error for internal_only dataset without entitlement")
	}

	// Grant entitlement
	ent := &Entitlement{
		Dataset:         DatasetDailyOHLCV,
		CanRedistribute: true,
	}
	err = CheckRedistributionPermission(DatasetDailyOHLCV, ent)
	if err != nil {
		t.Errorf("expected permitted with explicit entitlement, got: %v", err)
	}
}

func TestExportFormatting(t *testing.T) {
	headers := []string{"symbol", "open", "high", "low", "close", "volume"}
	rows := [][]string{
		{"NABIL", "500", "520", "495", "510", "25000"},
		{"SCB", "400", "410", "395", "405", "18000"},
	}

	// 1. CSV Format
	csvBytes, err := FormatExport(DatasetDailyOHLCV, FormatCSV, headers, rows)
	if err != nil {
		t.Fatalf("FormatExport CSV failed: %v", err)
	}
	csvStr := string(csvBytes)
	if !strings.Contains(csvStr, "# LICENSE: public_unverified") {
		t.Error("CSV missing licensing header")
	}
	if !strings.Contains(csvStr, "NABIL,500,520,495,510,25000") {
		t.Error("CSV missing data row")
	}

	// 2. JSON Format
	jsonBytes, err := FormatExport(DatasetCompanies, FormatJSON, headers, rows)
	if err != nil {
		t.Fatalf("FormatExport JSON failed: %v", err)
	}
	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"dataset": "companies"`) {
		t.Error("JSON missing dataset metadata")
	}
	if !strings.Contains(jsonStr, `"symbol": "NABIL"`) {
		t.Error("JSON missing structured data item")
	}

	// 3. Parquet Format
	parquetBytes, err := FormatExport(DatasetFloorsheet, FormatParquet, headers, rows)
	if err != nil {
		t.Fatalf("FormatExport Parquet failed: %v", err)
	}
	if !strings.HasPrefix(string(parquetBytes), "PAR1") {
		t.Error("Parquet missing PAR1 magic header")
	}
	if !strings.HasSuffix(string(parquetBytes), "PAR1") {
		t.Error("Parquet missing PAR1 magic footer")
	}
}
