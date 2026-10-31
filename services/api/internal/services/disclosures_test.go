package services

import (
	"testing"
	"time"
)

func TestDisclosureValidation(t *testing.T) {
	disc := &Disclosure{
		Authority:       AuthorityNRB,
		Category:        CategoryDirective,
		Title:           "Unified Directive 2080 Amendment on Margin Lending",
		Summary:         "Nepal Rastra Bank increases single borrower margin lending ceiling",
		Body:            "NRB circular number 14/2080-81 directs all commercial banks...",
		PublishedAt:     time.Date(2024, 5, 10, 10, 0, 0, 0, NepalLocation),
		EffectiveDate:   "2024-05-15",
		SourceURL:       "https://nrb.org.np/circulars/14-2080-81.pdf",
		ReferenceNumber: "Circular-14/2080-81",
		LicenseClass:    LicenseOfficialVerified,
		Redistribution:  RedistributePermittedWithAttribution,
		IsVerified:      true,
		Attachments: []Attachment{
			{
				FileName: "circular14.pdf",
				FileURL:  "https://nrb.org.np/docs/circular14.pdf",
				SHA256:   "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			},
		},
	}

	if err := disc.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if disc.PublishedAtNPT == "" {
		t.Errorf("expected PublishedAtNPT to be populated")
	}
}

func TestCompanyDisclosureRequiresSymbol(t *testing.T) {
	disc := &Disclosure{
		Authority: AuthorityCompany,
		Category:  CategoryDividendNotice,
		Title:     "Dividend Announcement",
	}

	err := disc.Validate()
	if err == nil {
		t.Fatal("expected error for company disclosure without symbol")
	}
}

func TestNepalDateParsing(t *testing.T) {
	utc, err := ParseNepalDate("2024-05-10 15:45:00")
	if err != nil {
		t.Fatalf("ParseNepalDate failed: %v", err)
	}

	// 15:45 in Nepal (UTC+05:45) is 10:00:00 UTC
	if utc.Hour() != 10 || utc.Minute() != 0 {
		t.Errorf("expected 10:00 UTC, got %02d:%02d", utc.Hour(), utc.Minute())
	}
}
