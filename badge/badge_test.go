package badge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSVGPass(t *testing.T) {
	svg := SVG("OJS conformance", "L0-L4", "pass")
	if !strings.Contains(svg, "<svg") {
		t.Error("expected SVG output")
	}
	if !strings.Contains(svg, "#4c1") {
		t.Error("expected green color for pass")
	}
	if !strings.Contains(svg, "L0-L4") {
		t.Error("expected level text")
	}
}

func TestSVGFail(t *testing.T) {
	svg := SVG("OJS conformance", "L0", "fail")
	if !strings.Contains(svg, "#e05d44") {
		t.Error("expected red color for fail")
	}
}

func TestSVGPartial(t *testing.T) {
	svg := SVG("OJS test", "L0-L2", "partial")
	if !strings.Contains(svg, "#dfb317") {
		t.Error("expected yellow color for partial")
	}
}

func TestServeBadge(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest("GET", "/badge/L0-L4.svg?name=Redis&status=pass", nil)
	rec := httptest.NewRecorder()
	h.ServeBadge(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("expected SVG content type, got %s", ct)
	}
	if !strings.Contains(rec.Body.String(), "OJS Redis") {
		t.Error("expected backend name in badge")
	}
}

func TestServeBadgeDefaults(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest("GET", "/badge/.svg", nil)
	rec := httptest.NewRecorder()
	h.ServeBadge(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestServeStatus(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest("GET", "/status", nil)
	rec := httptest.NewRecorder()
	h.ServeStatus(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Conformance Badge Service") {
		t.Error("expected service name")
	}
}
