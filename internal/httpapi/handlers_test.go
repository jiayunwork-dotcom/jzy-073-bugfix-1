package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"isa-service/internal/isa"
)

func setup() http.Handler { return NewRouter() }

func getJSON(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	setup().ServeHTTP(rec, req)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (body: %s)", err, rec.Body.String())
	}
	return rec.Code, body
}

func TestPointEndpointSeaLevel(t *testing.T) {
	code, body := getJSON(t, "/api/isa/point?altitude=0")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %v", code, body)
	}
	if math.Abs(body["temperature_k"].(float64)-288.15) > 1e-9 {
		t.Errorf("temperature_k = %v, want 288.15", body["temperature_k"])
	}
	if math.Abs(body["pressure_pa"].(float64)-101325) > 1e-6 {
		t.Errorf("pressure_pa = %v, want 101325", body["pressure_pa"])
	}
	if math.Abs(body["density_kg_m3"].(float64)-1.225) > 1e-9 {
		t.Errorf("density_kg_m3 = %v, want 1.225", body["density_kg_m3"])
	}
}

func TestPointEndpointWithOffset(t *testing.T) {
	code, body := getJSON(t, "/api/isa/point?altitude=10000&delta_t=15")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %v", code, body)
	}
	if math.Abs(body["temperature_offset_k"].(float64)-15) > 1e-9 {
		t.Errorf("offset = %v, want 15", body["temperature_offset_k"])
	}
	// Pressure must be the standard profile value at 10 km.
	std, err := isa.Model(10000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(body["pressure_pa"].(float64)-std.Pressure) > 1e-6 {
		t.Errorf("pressure with offset = %v, want standard %.6f", body["pressure_pa"], std.Pressure)
	}
	// Density altitude must exceed the geometric altitude for warm air.
	if body["density_altitude_m"].(float64) <= 10000 {
		t.Errorf("warm density altitude = %v, want > 10000", body["density_altitude_m"])
	}
}

func TestPointEndpointInvalidAltitude(t *testing.T) {
	for _, p := range []string{
		"/api/isa/point?altitude=-5",
		"/api/isa/point?altitude=25000",
		"/api/isa/point",
		"/api/isa/point?altitude=abc",
	} {
		code, body := getJSON(t, p)
		if code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", p, code)
		}
		reason, _ := body["reason"].(string)
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s: structured error reason missing: %v", p, body)
		}
		if body["error"] != "invalid_request" {
			t.Errorf("%s: error code = %v, want invalid_request", p, body["error"])
		}
	}
}

func TestProfileEndpoint(t *testing.T) {
	code, body := getJSON(t, "/api/isa/profile?start=0&end=3000&step=1000")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %v", code, body)
	}
	if body["count"].(float64) != 4 {
		t.Errorf("count = %v, want 4", body["count"])
	}
	points := body["points"].([]any)
	last := points[3].(map[string]any)
	if math.Abs(last["altitude_m"].(float64)-3000) > 1e-9 {
		t.Errorf("last altitude = %v, want 3000", last["altitude_m"])
	}

	// Invalid interval is rejected structurally.
	if code, body := getJSON(t, "/api/isa/profile?start=0&end=50000&step=1000"); code != http.StatusBadRequest {
		t.Errorf("ceiling breach: status = %d, want 400 (%v)", code, body)
	}
	if code, body := getJSON(t, "/api/isa/profile?start=1000&end=0&step=100"); code != http.StatusBadRequest {
		t.Errorf("inverted interval: status = %d, want 400 (%v)", code, body)
	}
}

func TestDemoEndpoint(t *testing.T) {
	code, body := getJSON(t, "/api/isa/demo")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %v", code, body)
	}
	atm := body["atmosphere"].(map[string]any)
	if math.Abs(atm["temperature_k"].(float64)-216.65) > 1e-9 {
		t.Errorf("demo temperature = %v, want 216.65 K", atm["temperature_k"])
	}
	if math.Abs(atm["pressure_pa"].(float64)-22632.06)/22632.06 > 1e-4 {
		t.Errorf("demo pressure = %v, want ~22632.06 Pa", atm["pressure_pa"])
	}
	if math.Abs(atm["altitude_m"].(float64)-11000) > 1e-9 {
		t.Errorf("demo altitude = %v, want 11000 m", atm["altitude_m"])
	}
}

func TestHealthz(t *testing.T) {
	code, _ := getJSON(t, "/healthz")
	if code != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", code)
	}
}

// 单点：极冷偏差使密度高度反解落到海平面以下，必须 400 + 结构化越界错误，
// 绝不能在响应体里塞一个负密度高度的“成功”结果。
func TestPointEndpointDensityAltitudeBelowFloor(t *testing.T) {
	code, body := getJSON(t, "/api/isa/point?altitude=0&delta_t=-100")
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body = %v)", code, body)
	}
	if body["error"] != "out_of_model_range" {
		t.Errorf("error = %v, want out_of_model_range", body["error"])
	}
	if reason, _ := body["reason"].(string); reason == "" {
		t.Errorf("structured reason missing: %v", body)
	}
	if body["direction"] != "below_sea_level" {
		t.Errorf("direction = %v, want below_sea_level", body["direction"])
	}
	if _, present := body["density_altitude_m"]; present {
		t.Errorf("out-of-range response must not carry a fabricated density altitude: %v", body)
	}
}

// 单点：极热偏差使密度高度反解落到 20 km 以上，同样 400 + 结构化越界错误。
func TestPointEndpointDensityAltitudeAboveCeiling(t *testing.T) {
	code, body := getJSON(t, "/api/isa/point?altitude=20000&delta_t=50")
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body = %v)", code, body)
	}
	if body["error"] != "out_of_model_range" {
		t.Errorf("error = %v, want out_of_model_range", body["error"])
	}
	if reason, _ := body["reason"].(string); reason == "" {
		t.Errorf("structured reason missing: %v", body)
	}
	if body["direction"] != "above_ceiling" {
		t.Errorf("direction = %v, want above_ceiling", body["direction"])
	}
}

// 边界检查不能误伤合法请求：很深的偏差只要密度高度仍落在模型范围内，
// 照常 200 且返回一个远离几何高度但合法的密度高度。
func TestPointEndpointExtremeButInRangeOffsetStillSucceeds(t *testing.T) {
	// h = 10000 m with -100 K: equivalent altitude ~6.2 km, in range.
	code, body := getJSON(t, "/api/isa/point?altitude=10000&delta_t=-100")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body = %v)", code, body)
	}
	da := body["density_altitude_m"].(float64)
	if da < 0 || da > 20000 {
		t.Errorf("in-range density altitude = %v, want within [0,20000]", da)
	}
	// h = 0 m with +250 K: equivalent altitude still far below the ceiling.
	code, body = getJSON(t, "/api/isa/point?altitude=0&delta_t=250")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body = %v)", code, body)
	}
	da = body["density_altitude_m"].(float64)
	if da < 0 || da > 20000 {
		t.Errorf("in-range density altitude = %v, want within [0,20000]", da)
	}
}

// 批量，偏冷方向：只有越界采样点失败（结构化原因），其余点保持正常，
// 整批仍 200 且 failed_count 只数失败点。
func TestProfileEndpointPerPointFailureColdSide(t *testing.T) {
	code, body := getJSON(t, "/api/isa/profile?start=0&end=20000&step=10000&delta_t=-100")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with per-point failure (body = %v)", code, body)
	}
	if body["failed_count"].(float64) != 1 {
		t.Errorf("failed_count = %v, want 1", body["failed_count"])
	}
	points := body["points"].([]any)
	if len(points) != 3 {
		t.Fatalf("points length = %d, want 3 (bad sample must keep its slot)", len(points))
	}

	bad := points[0].(map[string]any)
	if bad["status"] != "out_of_model_range" || bad["error"] != "out_of_model_range" {
		t.Errorf("bad point shape wrong: %v", bad)
	}
	if bad["direction"] != "below_sea_level" {
		t.Errorf("bad point direction = %v, want below_sea_level", bad["direction"])
	}
	if reason, _ := bad["reason"].(string); reason == "" {
		t.Errorf("bad point structured reason missing: %v", bad)
	}
	if bad["altitude_m"].(float64) != 0 || bad["index"].(float64) != 0 {
		t.Errorf("bad point identity wrong: %v", bad)
	}
	if _, present := bad["density_altitude_m"]; present {
		t.Errorf("bad point must not carry a fabricated density altitude: %v", bad)
	}

	// The two legal points keep the exact success shape.
	for i := 1; i <= 2; i++ {
		p := points[i].(map[string]any)
		if _, present := p["status"]; present {
			t.Errorf("good point %d must keep the plain success shape: %v", i, p)
		}
		if _, present := p["density_altitude_m"]; !present {
			t.Errorf("good point %d missing density_altitude_m: %v", i, p)
		}
		h := p["altitude_m"].(float64)
		direct, err := isa.Model(h, -100)
		if err != nil {
			t.Fatalf("direct Model(%v,-100) error: %v", h, err)
		}
		if math.Abs(p["pressure_pa"].(float64)-direct.Pressure) > 1e-6 {
			t.Errorf("good point %d pressure disagrees with direct Model", i)
		}
	}
}

// 批量，偏热方向：只有 20 km 采样点失败，其余正常，且槽位/索引不丢。
func TestProfileEndpointPerPointFailureHotSide(t *testing.T) {
	code, body := getJSON(t, "/api/isa/profile?start=0&end=20000&step=10000&delta_t=50")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with per-point failure (body = %v)", code, body)
	}
	if body["failed_count"].(float64) != 1 {
		t.Errorf("failed_count = %v, want 1", body["failed_count"])
	}
	points := body["points"].([]any)
	bad := points[2].(map[string]any)
	if bad["status"] != "out_of_model_range" || bad["direction"] != "above_ceiling" {
		t.Errorf("bad point shape/direction wrong: %v", bad)
	}
	if bad["altitude_m"].(float64) != 20000 || bad["index"].(float64) != 2 {
		t.Errorf("bad point identity wrong: %v", bad)
	}
	// Neighbouring legal points are untouched.
	for i := 0; i <= 1; i++ {
		p := points[i].(map[string]any)
		if _, present := p["error"]; present {
			t.Errorf("point %d unexpectedly marked failed: %v", i, p)
		}
	}
}

// 批量全部合法（即使偏差很深、密度高度离几何高度很远）：failed_count 必须为 0。
func TestProfileEndpointAllInRangeHasNoFailures(t *testing.T) {
	// 5..15 km with -100 K: every equivalent altitude stays well inside
	// [0, 20000 m] even though it sits far below the geometric altitude.
	code, body := getJSON(t, "/api/isa/profile?start=5000&end=15000&step=5000&delta_t=-100")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body = %v)", code, body)
	}
	if body["failed_count"].(float64) != 0 {
		t.Errorf("failed_count = %v, want 0", body["failed_count"])
	}
	for i, pt := range body["points"].([]any) {
		p := pt.(map[string]any)
		if _, present := p["error"]; present {
			t.Errorf("point %d unexpectedly failed: %v", i, p)
		}
	}
}
