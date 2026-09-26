package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// 联调复现：0 m 叠 -100 K（模拟极寒），空气比海平面标准还稠，反解出的
// 密度高度约 -4670 m。必须 400 + 结构化原因，响应里绝不能出现野值。
func TestPointDensityAltitudeBelowSeaLevel(t *testing.T) {
	code, body := getJSON(t, "/api/isa/point?altitude=0&delta_t=-100")
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %v)", code, body)
	}
	if body["error"] != "invalid_request" {
		t.Errorf("error = %v, want invalid_request", body["error"])
	}
	reason, _ := body["reason"].(string)
	if !strings.Contains(reason, "density altitude") {
		t.Errorf("reason should identify the density altitude as out of range, got: %q", reason)
	}
	if _, leaked := body["density_altitude_m"]; leaked {
		t.Errorf("wild density altitude leaked into response: %v", body)
	}
}

// 反方向：20000 m 叠 +1000 K，空气比模型顶层还稀，反解高度约 30943 m。
func TestPointDensityAltitudeAboveCeiling(t *testing.T) {
	code, body := getJSON(t, "/api/isa/point?altitude=20000&delta_t=1000")
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %v)", code, body)
	}
	if body["error"] != "invalid_request" {
		t.Errorf("error = %v, want invalid_request", body["error"])
	}
	reason, _ := body["reason"].(string)
	if !strings.Contains(reason, "density altitude") {
		t.Errorf("reason should identify the density altitude as out of range, got: %q", reason)
	}
	if _, leaked := body["density_altitude_m"]; leaked {
		t.Errorf("wild density altitude leaked into response: %v", body)
	}
}

// 防误伤回归：反解高度落在 [0, 20000] 内的偏移请求必须照常 200，
// 哪怕密度高度和几何高度差得很远。
func TestPointInRangeOffsetsStillOK(t *testing.T) {
	for _, p := range []string{
		"/api/isa/point?altitude=8000&delta_t=-15",   // 密度高度 ~7435 m
		"/api/isa/point?altitude=11000&delta_t=-100", // 密度高度 ~5781 m
		"/api/isa/point?altitude=15000&delta_t=15",   // 密度高度 ~15425 m
		"/api/isa/point?altitude=0&delta_t=100",      // 密度高度 ~2997 m
		"/api/isa/point?altitude=20000&delta_t=-50",  // 密度高度 ~18336 m
	} {
		code, body := getJSON(t, p)
		if code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (%v)", p, code, body)
			continue
		}
		da, ok := body["density_altitude_m"].(float64)
		if !ok {
			t.Errorf("%s: missing density_altitude_m in %v", p, body)
			continue
		}
		if da < 0 || da > 20000 {
			t.Errorf("%s: density altitude %v outside [0, 20000]", p, da)
		}
	}
}

// 批量接口（冷侧）：delta_t=-100 时 0/2000/4000 m 越界单独判废，
// 6000/8000/10000 m 照常返回，整批不判废。
func TestProfileDensityAltitudeBelowSeaLevel(t *testing.T) {
	code, body := getJSON(t, "/api/isa/profile?start=0&end=10000&step=2000&delta_t=-100")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (one bad point must not fail the batch): %v", code, body)
	}
	if body["count"].(float64) != 6 {
		t.Fatalf("count = %v, want 6", body["count"])
	}
	if body["failures"].(float64) != 3 {
		t.Errorf("failures = %v, want 3", body["failures"])
	}
	points := body["points"].([]any)

	// 越界点：结构化失败，带高度和清楚原因，不带野值。
	for i, want := range []float64{0, 2000, 4000} {
		p := points[i].(map[string]any)
		if p["altitude_m"].(float64) != want {
			t.Errorf("failed point %d altitude = %v, want %v", i, p["altitude_m"], want)
		}
		if p["error"] != "invalid_request" {
			t.Errorf("point %d error = %v, want invalid_request", i, p["error"])
		}
		if reason, _ := p["reason"].(string); !strings.Contains(reason, "density altitude") {
			t.Errorf("point %d reason should identify the density altitude, got %q", i, reason)
		}
		if _, leaked := p["density_altitude_m"]; leaked {
			t.Errorf("point %d leaked a wild density altitude: %v", i, p)
		}
	}

	// 合法点：完整正常结果，不受同批越界点牵连。
	for i, want := range []float64{6000, 8000, 10000} {
		p := points[i+3].(map[string]any)
		if p["altitude_m"].(float64) != want {
			t.Errorf("ok point %d altitude = %v, want %v", i, p["altitude_m"], want)
		}
		da, ok := p["density_altitude_m"].(float64)
		if !ok {
			t.Errorf("ok point %d missing density_altitude_m: %v", i, p)
			continue
		}
		if da < 0 || da > 20000 {
			t.Errorf("ok point %d density altitude %v outside [0, 20000]", i, da)
		}
		if _, ok := p["temperature_k"]; !ok {
			t.Errorf("ok point %d missing atmosphere fields: %v", i, p)
		}
	}
}

// 批量接口（热侧）：delta_t=+1000 时 10000/15000/20000 m 越界单独判废，
// 0/5000 m 照常返回。
func TestProfileDensityAltitudeAboveCeiling(t *testing.T) {
	code, body := getJSON(t, "/api/isa/profile?start=0&end=20000&step=5000&delta_t=1000")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (one bad point must not fail the batch): %v", code, body)
	}
	if body["count"].(float64) != 5 {
		t.Fatalf("count = %v, want 5", body["count"])
	}
	if body["failures"].(float64) != 3 {
		t.Errorf("failures = %v, want 3", body["failures"])
	}
	points := body["points"].([]any)

	for i, want := range []float64{0, 5000} {
		p := points[i].(map[string]any)
		if p["altitude_m"].(float64) != want {
			t.Errorf("ok point %d altitude = %v, want %v", i, p["altitude_m"], want)
		}
		da, ok := p["density_altitude_m"].(float64)
		if !ok {
			t.Errorf("ok point %d should be a full atmosphere: %v", i, p)
			continue
		}
		if da < 0 || da > 20000 {
			t.Errorf("ok point %d density altitude %v outside [0, 20000]", i, da)
		}
	}
	for i, want := range []float64{10000, 15000, 20000} {
		p := points[i+2].(map[string]any)
		if p["altitude_m"].(float64) != want {
			t.Errorf("failed point %d altitude = %v, want %v", i, p["altitude_m"], want)
		}
		if p["error"] != "invalid_request" {
			t.Errorf("point %d error = %v, want invalid_request", i, p["error"])
		}
		if reason, _ := p["reason"].(string); !strings.Contains(reason, "density altitude") {
			t.Errorf("point %d reason should identify the density altitude, got %q", i, reason)
		}
		if _, leaked := p["density_altitude_m"]; leaked {
			t.Errorf("point %d leaked a wild density altitude: %v", i, p)
		}
	}
}
