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
