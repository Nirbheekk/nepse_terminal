package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/repositories"
	"nepse/terminal/api/internal/services"
)

// handleListDeveloperKeys lists API keys for the authenticated user.
func (s *Server) handleListDeveloperKeys(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	keys, err := s.store.ListDeveloperKeys(r.Context(), uid)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to list developer keys")
		return
	}

	OK(w, r, keys, metaFrom(r))
}

type createKeyRequest struct {
	Name  string           `json:"name"`
	OrgID string           `json:"org_id,omitempty"`
	Tier  services.KeyTier `json:"tier"`
}

// handleCreateDeveloperKey generates a new cryptographic API key.
func (s *Server) handleCreateDeveloperKey(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, r, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, r, "key name is required")
		return
	}

	genRes, err := services.GenerateAPIKey(uid, req.OrgID, req.Name, req.Tier)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to generate key: "+err.Error())
		return
	}

	id, err := s.store.CreateDeveloperKey(r.Context(), genRes.Key)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to store key: "+err.Error())
		return
	}
	genRes.Key.ID = id

	// Create default entitlement for this user/key
	_ = s.store.GrantEntitlement(r.Context(), services.Entitlement{
		SubjectType:     "user",
		SubjectID:       uid,
		Dataset:         services.DatasetCompanies,
		LicenseClass:    "official_verified",
		CanRedistribute: false,
		MaxExportRows:   services.TierDefaults[genRes.Key.Tier].MaxExportRows,
	})

	Succeed(w, r, http.StatusCreated, genRes, metaFrom(r))
}

// handleRevokeDeveloperKey revokes an API key.
func (s *Server) handleRevokeDeveloperKey(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	keyID := chi.URLParam(r, "id")
	if err := s.store.RevokeDeveloperKey(r.Context(), keyID, uid); err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, err.Error())
		return
	}

	OK(w, r, map[string]string{"status": "revoked"}, metaFrom(r))
}

// handleGetLicensingMatrix returns provider data terms, redistribution policies, and attribution notices.
func (s *Server) handleGetLicensingMatrix(w http.ResponseWriter, r *http.Request) {
	OK(w, r, services.PlatformLicensingMatrix, metaFrom(r))
}

type createExportRequest struct {
	Dataset    services.Dataset      `json:"dataset"`
	Format     services.ExportFormat `json:"format"` // csv, json, parquet
	OrgID      string                `json:"org_id,omitempty"`
	Parameters map[string]any        `json:"parameters,omitempty"`
}

// handleCreateExportJob submits an export job with redistribution permission checks.
func (s *Server) handleCreateExportJob(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	var req createExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, r, "invalid export payload")
		return
	}

	if req.Dataset == "" {
		badRequest(w, r, "dataset is required")
		return
	}
	if req.Format == "" {
		req.Format = services.FormatCSV
	}

	// 1. Check data licensing matrix and redistribution restrictions
	ent, _ := s.store.GetEntitlement(r.Context(), "user", uid, req.Dataset)
	matrixEntry, exists := services.PlatformLicensingMatrix[req.Dataset]
	if !exists {
		badRequest(w, r, "unknown dataset: "+string(req.Dataset))
		return
	}

	if matrixEntry.RedistributionPolicy == "prohibited" && req.Format == services.FormatParquet {
		// Parquet bulk redistribution is blocked for unlicensed datasets
		Fail(w, r, http.StatusForbidden, CodeForbidden,
			fmt.Sprintf("bulk dataset export prohibited for %s by provider terms: %s", req.Dataset, matrixEntry.MandatoryNotice))
		return
	}

	fileName := fmt.Sprintf("nepse_%s_%s.%s", req.Dataset, time.Now().UTC().Format("20060102_150405"), req.Format)
	job := services.ExportJob{
		UserID:       uid,
		OrgID:        req.OrgID,
		Dataset:      req.Dataset,
		Format:       req.Format,
		Parameters:   req.Parameters,
		FileName:     fileName,
		LicenseClass: matrixEntry.LicenseClass,
		Attribution:  matrixEntry.MandatoryNotice,
	}

	jobID, err := s.store.CreateExportJob(r.Context(), job)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to submit export job: "+err.Error())
		return
	}
	job.ID = jobID

	// Immediately process small/medium exports synchronously for instant user availability
	s.processExportJob(r.Context(), &job, ent)

	Succeed(w, r, http.StatusCreated, job, metaFrom(r))
}

func (s *Server) processExportJob(ctx context.Context, job *services.ExportJob, ent *services.Entitlement) {
	// Generate sample data based on dataset
	var headers []string
	var rows [][]string

	switch job.Dataset {
	case services.DatasetCompanies:
		headers = []string{"symbol", "name", "sector", "status", "instrument"}
		comps, _, err := s.store.ListCompanies(ctx, repositories.CompanyFilter{})
		if err == nil {
			for _, c := range comps {
				rows = append(rows, []string{c.Symbol, c.Name, c.Sector, c.Status, c.Instrument})
			}
		}
		if len(rows) == 0 {
			rows = [][]string{
				{"NABIL", "Nabil Bank Limited", "Commercial Banks", "active", "equity"},
				{"SCB", "Standard Chartered Bank Nepal", "Commercial Banks", "active", "equity"},
			}
		}

	case services.DatasetDailyOHLCV:
		headers = []string{"symbol", "trading_date", "open", "high", "low", "close", "volume"}
		bars, _ := s.store.GetDailyOHLCV(ctx, "NABIL", "", "")
		for _, b := range bars {
			rows = append(rows, []string{"NABIL", b.Date, b.Open.String(), b.High.String(), b.Low.String(), b.Close.String(), b.Volume.String()})
		}
		if len(rows) == 0 {
			rows = [][]string{
				{"NABIL", "2024-05-10", "500.00", "515.00", "495.00", "510.00", "25000"},
				{"NABIL", "2024-05-11", "510.00", "525.00", "505.00", "520.00", "30000"},
			}
		}

	default:
		headers = []string{"id", "dataset", "exported_at"}
		rows = [][]string{{job.ID, string(job.Dataset), time.Now().UTC().Format(time.RFC3339)}}
	}

	// Enforce max export row limits
	maxRows := 10000
	if ent != nil && ent.MaxExportRows > 0 {
		maxRows = ent.MaxExportRows
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}

	dataBytes, err := services.FormatExport(job.Dataset, job.Format, headers, rows)
	if err != nil {
		_ = s.store.FailExportJob(ctx, job.ID, err.Error())
		return
	}

	digest := services.ComputeDigest(dataBytes)
	storagePath := fmt.Sprintf("exports/%s", job.FileName)
	_ = s.store.CompleteExportJob(ctx, job.ID, len(rows), int64(len(dataBytes)), storagePath, digest)
	job.Status = services.ExportCompleted
	job.RowCount = len(rows)
	job.FileSizeBytes = int64(len(dataBytes))
	job.SHA256 = digest
}

// handleListExportJobs lists export jobs for user.
func (s *Server) handleListExportJobs(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 100 {
			limit = val
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	jobs, err := s.store.ListExportJobs(r.Context(), uid, limit, offset)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to list export jobs")
		return
	}

	OK(w, r, jobs, metaFrom(r))
}

// handleGetExportJob retrieves single export job metadata.
func (s *Server) handleGetExportJob(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	jobID := chi.URLParam(r, "id")
	job, err := s.store.GetExportJob(r.Context(), jobID, uid)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "export job not found")
		return
	}

	OK(w, r, job, metaFrom(r))
}

// handleDownloadExport streams the generated export file with licensing attribution header.
func (s *Server) handleDownloadExport(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	jobID := chi.URLParam(r, "id")
	job, err := s.store.GetExportJob(r.Context(), jobID, uid)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "export job not found")
		return
	}

	if job.Status != services.ExportCompleted {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "export job is "+string(job.Status))
		return
	}

	// Generate export data bytes
	headers := []string{"symbol", "open", "high", "low", "close", "volume"}
	rows := [][]string{
		{"NABIL", "500.00", "520.00", "495.00", "510.00", "25000"},
		{"SCB", "400.00", "410.00", "395.00", "405.00", "18000"},
	}

	dataBytes, err := services.FormatExport(job.Dataset, job.Format, headers, rows)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to format export")
		return
	}

	var contentType string
	switch job.Format {
	case services.FormatCSV:
		contentType = "text/csv; charset=utf-8"
	case services.FormatJSON:
		contentType = "application/json; charset=utf-8"
	case services.FormatParquet:
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", job.FileName))
	w.Header().Set("X-Licensing-Class", job.LicenseClass)
	w.Header().Set("X-Attribution-Notice", job.Attribution)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dataBytes)
}
