package badge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPortalCertify(t *testing.T) {
	p := NewPortal()
	body := `{"server_url":"http://my-ojs:8080","name":"MyBackend","organization":"AcmeCorp"}`
	req := httptest.NewRequest("POST", "/api/certify", strings.NewReader(body))
	rec := httptest.NewRecorder()

	p.HandleCertify(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "queued" {
		t.Errorf("expected queued status, got %v", resp["status"])
	}
	if resp["certificate_id"] == "" {
		t.Error("expected non-empty certificate_id")
	}
}

func TestPortalCertifyMissingFields(t *testing.T) {
	p := NewPortal()

	// Missing server_url
	body := `{"name":"Test"}`
	req := httptest.NewRequest("POST", "/api/certify", strings.NewReader(body))
	rec := httptest.NewRecorder()
	p.HandleCertify(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing server_url, got %d", rec.Code)
	}

	// Missing name
	body = `{"server_url":"http://example.com"}`
	req = httptest.NewRequest("POST", "/api/certify", strings.NewReader(body))
	rec = httptest.NewRecorder()
	p.HandleCertify(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", rec.Code)
	}
}

func TestPortalGetCertificate(t *testing.T) {
	p := NewPortal()
	cert := p.store.Issue(CertificationRequest{
		ServerURL: "http://test:8080",
		Name:      "TestBackend",
	}, 170, 5, 175)

	req := httptest.NewRequest("GET", "/api/certificates/"+cert.ID, nil)
	req.SetPathValue("id", cert.ID)
	rec := httptest.NewRecorder()

	p.HandleGetCertificate(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var got Certificate
	json.NewDecoder(rec.Body).Decode(&got)
	if got.Name != "TestBackend" {
		t.Errorf("expected TestBackend, got %s", got.Name)
	}
	if got.Passed != 170 {
		t.Errorf("expected 170 passed, got %d", got.Passed)
	}
}

func TestPortalGetCertificateNotFound(t *testing.T) {
	p := NewPortal()
	req := httptest.NewRequest("GET", "/api/certificates/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rec := httptest.NewRecorder()

	p.HandleGetCertificate(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPortalListCertificates(t *testing.T) {
	p := NewPortal()
	p.store.Issue(CertificationRequest{ServerURL: "http://a:8080", Name: "A"}, 175, 0, 175)
	p.store.Issue(CertificationRequest{ServerURL: "http://b:8080", Name: "B"}, 100, 75, 175)

	req := httptest.NewRequest("GET", "/api/certificates", nil)
	rec := httptest.NewRecorder()

	p.HandleListCertificates(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	count := int(resp["count"].(float64))
	if count != 2 {
		t.Errorf("expected 2 certificates, got %d", count)
	}
}

func TestPortalVerify(t *testing.T) {
	p := NewPortal()
	cert := p.store.Issue(CertificationRequest{
		ServerURL: "http://valid:8080",
		Name:      "ValidImpl",
	}, 175, 0, 175)

	// Valid verification
	req := httptest.NewRequest("GET", "/api/verify?id="+cert.ID+"&fingerprint="+cert.Fingerprint, nil)
	rec := httptest.NewRecorder()
	p.HandleVerify(rec, req)

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["valid"] != true {
		t.Error("expected valid=true for correct fingerprint")
	}

	// Invalid fingerprint
	req = httptest.NewRequest("GET", "/api/verify?id="+cert.ID+"&fingerprint=wrong", nil)
	rec = httptest.NewRecorder()
	p.HandleVerify(rec, req)

	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["valid"] != false {
		t.Error("expected valid=false for wrong fingerprint")
	}
}

func TestPortalVerifyMissingParams(t *testing.T) {
	p := NewPortal()
	req := httptest.NewRequest("GET", "/api/verify", nil)
	rec := httptest.NewRecorder()
	p.HandleVerify(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestUpdateCertificate(t *testing.T) {
	p := NewPortal()
	cert := p.store.Issue(CertificationRequest{
		ServerURL: "http://test:8080",
		Name:      "TestImpl",
	}, 0, 0, 0) // initially empty

	err := p.UpdateCertificate(cert.ID, 175, 0, 175)
	if err != nil {
		t.Fatalf("UpdateCertificate: %v", err)
	}

	updated, _ := p.store.Get(cert.ID)
	if updated.Passed != 175 {
		t.Errorf("expected 175 passed, got %d", updated.Passed)
	}
	if updated.Status != "pass" {
		t.Errorf("expected pass, got %s", updated.Status)
	}
	if updated.Level != "L0-L4" {
		t.Errorf("expected L0-L4, got %s", updated.Level)
	}
}

func TestComputeLevel(t *testing.T) {
	tests := []struct {
		passed int
		total  int
		want   string
	}{
		{175, 175, "L0-L4"},
		{140, 175, "L0-L3"},
		{105, 175, "L0-L2"},
		{70, 175, "L0-L1"},
		{30, 175, "L0"},
		{0, 0, "L0"},
	}

	for _, tt := range tests {
		got := computeLevel(tt.passed, tt.total)
		if got != tt.want {
			t.Errorf("computeLevel(%d, %d) = %s, want %s", tt.passed, tt.total, got, tt.want)
		}
	}
}
