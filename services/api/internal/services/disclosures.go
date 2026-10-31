// Package disclosures manages authorized disclosures and circulars from NEPSE, SEBON,
// NRB (Nepal Rastra Bank), and listed companies with attachment metadata and licensing provenance.
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Authority string

const (
	AuthorityNEPSE   Authority = "NEPSE"   // Nepal Stock Exchange
	AuthoritySEBON   Authority = "SEBON"   // Securities Board of Nepal
	AuthorityNRB     Authority = "NRB"     // Nepal Rastra Bank (Central Bank)
	AuthorityCompany Authority = "COMPANY" // Listed Issuer
)

type Category string

const (
	CategoryCircular           Category = "circular"
	CategoryDirective          Category = "directive"
	CategoryDividendNotice     Category = "dividend_notice"
	CategoryFinancialReport    Category = "financial_report"
	CategoryAGMNotice          Category = "agm_notice"
	CategorySuspension         Category = "suspension"
	CategoryListingNotice      Category = "listing_notice"
	CategoryIPOApproval        Category = "ipo_approval"
	CategoryMonetaryPolicy     Category = "monetary_policy"
	CategoryEnforcement        Category = "enforcement"
	CategoryMaterialDisclosure Category = "material_disclosure"
)

type LicenseClass string

const (
	LicenseOfficialVerified LicenseClass = "official_verified"
	LicenseThirdParty       LicenseClass = "third_party_licensed"
	LicensePublicUnverified LicenseClass = "public_unverified"
	LicenseDerived          LicenseClass = "derived"
	LicenseRestricted       LicenseClass = "restricted"
)

type RedistributionRights string

const (
	RedistributeProhibited               RedistributionRights = "prohibited"
	RedistributeInternalOnly             RedistributionRights = "internal_only"
	RedistributePermittedWithAttribution RedistributionRights = "permitted_with_attribution"
	RedistributeLicensed                 RedistributionRights = "licensed"
)

// NepalOffset is UTC+05:45.
var NepalLocation = time.FixedZone("NPT", 5*3600+45*60)

// Attachment represents an official document or circular attached to a disclosure.
type Attachment struct {
	ID            string `json:"id,omitempty"`
	DisclosureID  string `json:"disclosure_id,omitempty"`
	FileName      string `json:"file_name"`
	FileURL       string `json:"file_url"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	MimeType      string `json:"mime_type"`
	SHA256        string `json:"sha256"`
	StorageKey    string `json:"storage_key,omitempty"`
}

// Disclosure represents an authorized publication or circular.
type Disclosure struct {
	ID              string               `json:"id"`
	Authority       Authority            `json:"authority"`
	CompanyID       string               `json:"company_id,omitempty"`
	Symbol          string               `json:"symbol,omitempty"`
	Category        Category             `json:"category"`
	Title           string               `json:"title"`
	Summary         string               `json:"summary"`
	Body            string               `json:"body"`
	PublishedAt     time.Time            `json:"published_at"`
	PublishedAtNPT  string               `json:"published_at_npt"`         // YYYY-MM-DD HH:MM:SS NPT
	EffectiveDate   string               `json:"effective_date,omitempty"` // YYYY-MM-DD
	SourceURL       string               `json:"source_url"`
	ReferenceNumber string               `json:"reference_number"` // Circular number, dispatch ref
	LicenseClass    LicenseClass         `json:"license_class"`
	Redistribution  RedistributionRights `json:"redistribution"`
	IsVerified      bool                 `json:"is_verified"`
	Attachments     []Attachment         `json:"attachments,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
}

// Validate ensures all mandatory fields and provenance rules are satisfied.
func (d *Disclosure) Validate() error {
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" {
		return errors.New("disclosure title cannot be empty")
	}

	switch d.Authority {
	case AuthorityNEPSE, AuthoritySEBON, AuthorityNRB, AuthorityCompany:
	default:
		return fmt.Errorf("invalid authority: %q (must be NEPSE, SEBON, NRB, or COMPANY)", d.Authority)
	}

	if d.Authority == AuthorityCompany && d.Symbol == "" {
		return errors.New("symbol is required for company disclosures")
	}
	d.Symbol = strings.ToUpper(strings.TrimSpace(d.Symbol))

	if d.LicenseClass == "" {
		d.LicenseClass = LicenseOfficialVerified
	}
	if d.Redistribution == "" {
		d.Redistribution = RedistributePermittedWithAttribution
	}

	if d.PublishedAt.IsZero() {
		d.PublishedAt = time.Now().UTC()
	}

	// Compute Nepal time representation
	nptTime := d.PublishedAt.In(NepalLocation)
	d.PublishedAtNPT = nptTime.Format("2006-01-02 15:04:05 MST")

	// Validate attachments
	for i := range d.Attachments {
		att := &d.Attachments[i]
		if att.FileName == "" {
			return errors.New("attachment file_name cannot be empty")
		}
		if att.FileURL == "" {
			return errors.New("attachment file_url cannot be empty")
		}
		if att.MimeType == "" {
			att.MimeType = "application/pdf"
		}
		if att.SHA256 != "" && len(att.SHA256) != 64 {
			return fmt.Errorf("invalid sha256 checksum for attachment %s", att.FileName)
		}
	}

	return nil
}

// ComputeSHA256 computes a hex-encoded SHA-256 digest of document bytes.
func ComputeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ParseNepalDate parses a date string in Nepal time (YYYY-MM-DD or YYYY-MM-DD HH:MM:SS) and converts to UTC.
func ParseNepalDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		time.RFC3339,
	}

	for _, layout := range formats {
		if t, err := time.ParseInLocation(layout, s, NepalLocation); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse Nepal date: %q", s)
}
