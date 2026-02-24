// Package badge provides an HTTP service that generates OJS conformance badges.
//
// Third-party OJS implementations can request a conformance test run
// and receive an SVG badge showing their conformance level.
package badge

import (
	"fmt"
	"net/http"
	"strings"
)

// SVG generates an OJS conformance badge as SVG.
func SVG(label, level, status string) string {
	color := "#4c1"
	if status == "fail" {
		color = "#e05d44"
	} else if status == "partial" {
		color = "#dfb317"
	}

	labelWidth := len(label)*7 + 10
	valueWidth := len(level)*7 + 10
	totalWidth := labelWidth + valueWidth

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img">
  <linearGradient id="s" x2="0" y2="100%%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
  <clipPath id="r"><rect width="%d" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="20" fill="#555"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="11">
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
  </g>
</svg>`, totalWidth, totalWidth, labelWidth, labelWidth, valueWidth, color,
		totalWidth, labelWidth/2, label, labelWidth/2, label,
		labelWidth+valueWidth/2, level, labelWidth+valueWidth/2, level)
}

// Handler serves conformance badge HTTP endpoints.
type Handler struct{}

// NewHandler creates a badge HTTP handler.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeBadge handles GET /badge/{level}.svg — returns a conformance badge.
func (h *Handler) ServeBadge(w http.ResponseWriter, r *http.Request) {
	level := strings.TrimSuffix(r.URL.Path[len("/badge/"):], ".svg")
	if level == "" {
		level = "L0-L4"
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pass"
	}

	label := "OJS conformance"
	if name := r.URL.Query().Get("name"); name != "" {
		label = "OJS " + name
	}

	svg := SVG(label, level, status)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "max-age=300")
	w.Write([]byte(svg))
}

// ServeStatus handles GET /status — returns available badge configurations.
func (h *Handler) ServeStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{
  "service": "OJS Conformance Badge Service",
  "version": "1.0.0",
  "usage": {
    "badge_url": "/badge/{level}.svg?name={backend_name}&status={pass|fail|partial}",
    "examples": [
      "/badge/L0-L4.svg?name=MyBackend&status=pass",
      "/badge/L0-L2.svg?name=CustomImpl&status=partial",
      "/badge/L0.svg?status=fail"
    ],
    "levels": ["L0", "L0-L1", "L0-L2", "L0-L3", "L0-L4"]
  }
}`))
}
