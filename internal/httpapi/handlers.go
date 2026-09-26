// Package httpapi exposes the ISA model over HTTP using Gin.
package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"isa-service/internal/isa"
)

// errorBody is the single structured error shape returned by every failure.
type errorBody struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

func badRequest(c *gin.Context, reason string) {
	c.JSON(http.StatusBadRequest, errorBody{Error: "invalid_request", Reason: reason})
}

// parseFloat reads a float64 query parameter. required=false returns def when
// the parameter is absent; an unparsable value is a 400.
func parseFloat(c *gin.Context, name string, def float64, required bool) (float64, bool) {
	raw, present := c.GetQuery(name)
	if !present {
		if required {
			badRequest(c, "missing required query parameter: "+name)
			return 0, false
		}
		return def, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		badRequest(c, "query parameter "+name+" must be a number, got: "+raw)
		return 0, false
	}
	return v, true
}

// handlePoint serves GET /api/isa/point?altitude=<m>[&delta_t=<K>].
func handlePoint(c *gin.Context) {
	h, ok := parseFloat(c, "altitude", 0, true)
	if !ok {
		return
	}
	deltaT, _ := parseFloat(c, "delta_t", 0, false)

	a, err := isa.Model(h, deltaT)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, a)
}

// handleProfile serves GET /api/isa/profile?start=<m>&end=<m>&step=<m>[&delta_t=<K>].
func handleProfile(c *gin.Context) {
	start, ok := parseFloat(c, "start", 0, true)
	if !ok {
		return
	}
	end, ok := parseFloat(c, "end", 0, true)
	if !ok {
		return
	}
	step, ok := parseFloat(c, "step", 0, true)
	if !ok {
		return
	}
	deltaT, _ := parseFloat(c, "delta_t", 0, false)

	points, err := isa.Profile(start, end, step, deltaT)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"start_m":              start,
		"end_m":                end,
		"step_m":               step,
		"temperature_offset_k": deltaT,
		"count":                len(points),
		"points":               points,
	})
}

// handleDemo serves GET /api/isa/demo: the standard atmosphere at the
// 11 km tropopause, a commonly used cruise layer with hand-checkable values
// (T = 216.65 K, P ≈ 22632 Pa).
func handleDemo(c *gin.Context) {
	a, err := isa.Model(isa.TropopauseAltitude, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorBody{Error: "internal_error", Reason: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"description": "International Standard Atmosphere at the 11 km tropopause (a typical cruise layer); standard conditions, no temperature offset",
		"reference": gin.H{
			"temperature_k":      216.65,
			"pressure_pa":        22632.06,
			"density_kg_m3":      0.3639,
			"speed_of_sound_m_s": 295.1,
		},
		"atmosphere": a,
	})
}
