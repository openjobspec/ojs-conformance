package badge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

// --- Conformance Certification Portal ---

// CertificationRequest represents a request to certify an implementation.
type CertificationRequest struct {
	ServerURL      string `json:"server_url"`
	Name           string `json:"name"`
	Organization   string `json:"organization,omitempty"`
	Repository     string `json:"repository,omitempty"`
	Level          string `json:"level"` // "all" or "0"-"4"
	ContactEmail   string `json:"contact_email,omitempty"`
}

// Certificate represents a conformance certification result.
type Certificate struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Organization   string    `json:"organization,omitempty"`
	Repository     string    `json:"repository,omitempty"`
	Level          string    `json:"level"` // highest passing level: "L0", "L0-L1", etc.
	Status         string    `json:"status"` // "pass", "partial", "fail"
	Passed         int       `json:"passed"`
	Failed         int       `json:"failed"`
	Total          int       `json:"total"`
	BadgeURL       string    `json:"badge_url"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"` // re-certification required every 6 months
	Fingerprint    string    `json:"fingerprint"` // SHA-256 of cert data
}

// CertificationStore manages issued certificates.
type CertificationStore struct {
	mu    sync.RWMutex
	certs map[string]*Certificate // id -> cert
}

// NewCertificationStore creates a store for issued certificates.
func NewCertificationStore() *CertificationStore {
	return &CertificationStore{
		certs: make(map[string]*Certificate),
	}
}

// Issue creates and stores a new certificate.
func (cs *CertificationStore) Issue(req CertificationRequest, passed, failed, total int) *Certificate {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	level := computeLevel(passed, total)
	status := "pass"
	if failed > 0 && passed > 0 {
		status = "partial"
	}
	if passed == 0 {
		status = "fail"
	}

	now := time.Now()
	certData := fmt.Sprintf("%s:%s:%s:%d:%d:%s",
		req.Name, req.ServerURL, level, passed, total, now.Format(time.RFC3339))
	hash := sha256.Sum256([]byte(certData))
	fingerprint := hex.EncodeToString(hash[:])
	id := "cert_" + fingerprint[:16]

	cert := &Certificate{
		ID:           id,
		Name:         req.Name,
		Organization: req.Organization,
		Repository:   req.Repository,
		Level:        level,
		Status:       status,
		Passed:       passed,
		Failed:       failed,
		Total:        total,
		BadgeURL:     fmt.Sprintf("/badge/%s.svg?name=%s&status=%s", level, req.Name, status),
		IssuedAt:     now,
		ExpiresAt:    now.Add(180 * 24 * time.Hour), // 6 months
		Fingerprint:  fingerprint,
	}

	cs.certs[id] = cert
	return cert
}

// Get retrieves a certificate by ID.
func (cs *CertificationStore) Get(id string) (*Certificate, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	c, ok := cs.certs[id]
	return c, ok
}

// List returns all certificates, sorted by issue date (newest first).
func (cs *CertificationStore) List() []*Certificate {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	result := make([]*Certificate, 0, len(cs.certs))
	for _, c := range cs.certs {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].IssuedAt.After(result[j].IssuedAt)
	})
	return result
}

// Verify checks if a certificate fingerprint is valid.
func (cs *CertificationStore) Verify(id, fingerprint string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	c, ok := cs.certs[id]
	if !ok {
		return false
	}
	return c.Fingerprint == fingerprint && time.Now().Before(c.ExpiresAt)
}

// --- Portal HTTP Handlers ---

// Portal serves the conformance certification portal endpoints.
type Portal struct {
	store *CertificationStore
	badge *Handler
}

// NewPortal creates a certification portal.
func NewPortal() *Portal {
	return &Portal{
		store: NewCertificationStore(),
		badge: NewHandler(),
	}
}

// HandleCertify processes a certification request.
// POST /api/certify
func (p *Portal) HandleCertify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writePortalError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req CertificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePortalError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if req.ServerURL == "" {
		writePortalError(w, http.StatusBadRequest, "server_url is required")
		return
	}
	if req.Name == "" {
		writePortalError(w, http.StatusBadRequest, "name is required")
		return
	}

	// In production, this would run the actual conformance suite.
	// For now, return a placeholder that the async runner will fill in.
	cert := p.store.Issue(req, 0, 0, 0)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"certificate_id": cert.ID,
		"status":         "queued",
		"message":        "conformance test run has been queued",
	})
}

// HandleGetCertificate retrieves a certificate by ID.
// GET /api/certificates/{id}
func (p *Portal) HandleGetCertificate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writePortalError(w, http.StatusBadRequest, "certificate id required")
		return
	}

	cert, ok := p.store.Get(id)
	if !ok {
		writePortalError(w, http.StatusNotFound, "certificate not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cert)
}

// HandleListCertificates returns all issued certificates.
// GET /api/certificates
func (p *Portal) HandleListCertificates(w http.ResponseWriter, r *http.Request) {
	certs := p.store.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"certificates": certs,
		"count":        len(certs),
	})
}

// HandleVerify checks if a certificate is valid.
// GET /api/verify?id={id}&fingerprint={fp}
func (p *Portal) HandleVerify(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	fp := r.URL.Query().Get("fingerprint")

	if id == "" || fp == "" {
		writePortalError(w, http.StatusBadRequest, "id and fingerprint are required")
		return
	}

	valid := p.store.Verify(id, fp)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"valid":  valid,
		"id":     id,
	})
}

// RegisterRoutes registers portal endpoints on a standard ServeMux.
func (p *Portal) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/certify", p.HandleCertify)
	mux.HandleFunc("GET /api/certificates/{id}", p.HandleGetCertificate)
	mux.HandleFunc("GET /api/certificates", p.HandleListCertificates)
	mux.HandleFunc("GET /api/verify", p.HandleVerify)
	mux.HandleFunc("GET /badge/", p.badge.ServeBadge)
	mux.HandleFunc("GET /status", p.badge.ServeStatus)
}

// UpdateCertificate updates a certificate with test results (called after async run).
func (p *Portal) UpdateCertificate(id string, passed, failed, total int) error {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()

	cert, ok := p.store.certs[id]
	if !ok {
		return fmt.Errorf("certificate %s not found", id)
	}

	cert.Passed = passed
	cert.Failed = failed
	cert.Total = total
	cert.Level = computeLevel(passed, total)

	if failed == 0 && passed > 0 {
		cert.Status = "pass"
	} else if passed > 0 {
		cert.Status = "partial"
	} else {
		cert.Status = "fail"
	}

	cert.BadgeURL = fmt.Sprintf("/badge/%s.svg?name=%s&status=%s", cert.Level, cert.Name, cert.Status)
	return nil
}

func computeLevel(passed, total int) string {
	if total == 0 {
		return "L0"
	}
	ratio := float64(passed) / float64(total)
	if ratio >= 1.0 {
		return "L0-L4"
	}
	if ratio >= 0.8 {
		return "L0-L3"
	}
	if ratio >= 0.6 {
		return "L0-L2"
	}
	if ratio >= 0.4 {
		return "L0-L1"
	}
	return "L0"
}

func writePortalError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
