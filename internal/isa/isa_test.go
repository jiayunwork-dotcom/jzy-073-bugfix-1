package isa

import (
	"math"
	"testing"
)

const (
	absTol = 1e-9 // generic absolute tolerance for SI quantities
	relTol = 1e-9 // generic relative tolerance
)

func approx(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func approxRel(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol*math.Max(1.0, math.Max(math.Abs(a), math.Abs(b)))
}

// 高度为零时，温度、气压、密度必须精确回到海平面基准值。
func TestSeaLevelReferenceValues(t *testing.T) {
	a, err := Model(0, 0)
	if err != nil {
		t.Fatalf("Model(0,0) returned error: %v", err)
	}
	if !approx(a.Temperature, SeaLevelTemperature, absTol) {
		t.Errorf("sea-level temperature = %.10f K, want %v K", a.Temperature, SeaLevelTemperature)
	}
	if !approx(a.Pressure, SeaLevelPressure, absTol) {
		t.Errorf("sea-level pressure = %.10f Pa, want %v Pa", a.Pressure, SeaLevelPressure)
	}
	// Density must come back to the pinned reference (R is derived to guarantee this).
	if !approx(a.Density, SeaLevelDensity, 1e-12) {
		t.Errorf("sea-level density = %.12f kg/m^3, want %v kg/m^3", a.Density, SeaLevelDensity)
	}
	if a.SpeedOfSound <= 0 {
		t.Errorf("sea-level speed of sound non-positive: %v", a.SpeedOfSound)
	}
	// No offset: density altitude must equal geometric altitude at h = 0.
	if !approx(a.DensityAltitude, 0, 1e-9) {
		t.Errorf("sea-level density altitude = %.10f m, want 0", a.DensityAltitude)
	}
}

// 对流层里每升一公里，温度准确掉 6.5 K。
func TestTroposphereLapseRate(t *testing.T) {
	for km := 0; km < 11; km++ {
		lo, err1 := Model(float64(km)*1000, 0)
		hi, err2 := Model(float64(km+1)*1000, 0)
		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected errors at km %d: %v %v", km, err1, err2)
		}
		drop := lo.Temperature - hi.Temperature
		if !approx(drop, 6.5, 1e-9) {
			t.Errorf("temperature drop %dkm->%dkm = %.10f K, want 6.5 K", km, km+1, drop)
		}
	}
}

// 十一公里对流层顶的温度，必须等于等温层那个恒定温度值。
func TestTropopauseTemperatureIsIsothermalConstant(t *testing.T) {
	top, err := Model(TropopauseAltitude, 0)
	if err != nil {
		t.Fatalf("Model(11000,0) error: %v", err)
	}
	want := SeaLevelTemperature - LapseRate*TropopauseAltitude // 216.65 K
	if !approx(top.Temperature, want, absTol) {
		t.Errorf("tropopause temperature = %.10f K, want %.4f K", top.Temperature, want)
	}
	// Every point in the isothermal layer keeps exactly that temperature.
	for _, h := range []float64{11000, 12000, 15000, 18000, 20000} {
		a, err := Model(h, 0)
		if err != nil {
			t.Fatalf("Model(%v,0) error: %v", h, err)
		}
		if !approx(a.StandardTemperature, top.StandardTemperature, absTol) {
			t.Errorf("standard temperature at %v m = %.6f K, want constant %.6f K",
				h, a.StandardTemperature, top.StandardTemperature)
		}
	}
}

// 十一公里处两套公式的气压必须连续（且密度、声速也一致）。
func TestLayerBoundaryContinuity(t *testing.T) {
	pTropo := tropospherePressure(TropopauseAltitude)
	pIso := isothermalPressure(TropopauseAltitude)
	if !approx(pTropo, pIso, 1e-6) {
		t.Errorf("pressure jump at 11 km: troposphere = %.8f Pa, isothermal = %.8f Pa, diff = %.3e Pa",
			pTropo, pIso, pTropo-pIso)
	}
	if !approx(pTropo, pIso, 1e-12) {
		t.Logf("note: same-constant formulas agree to %.3e Pa", pTropo-pIso)
	}

	tTropo := troposphereTemperature(TropopauseAltitude)
	tIso := isothermalTemperature(TropopauseAltitude)
	if !approx(tTropo, tIso, absTol) {
		t.Errorf("temperature jump at 11 km: %.10f vs %.10f K", tTropo, tIso)
	}

	dTropo := troposphereDensity(TropopauseAltitude)
	dIso := isothermalDensity(TropopauseAltitude)
	if !approx(dTropo, dIso, 1e-9) {
		t.Errorf("density jump at 11 km: %.12f vs %.12f kg/m^3", dTropo, dIso)
	}
}

// 进了等温层，温度不变，气压和密度持续往下掉。
func TestIsothermalMonotonicDecrease(t *testing.T) {
	const step = 500.0
	prev, err := Model(TropopauseAltitude, 0)
	if err != nil {
		t.Fatal(err)
	}
	for h := TropopauseAltitude + step; h <= ModelCeiling; h += step {
		cur, err := Model(h, 0)
		if err != nil {
			t.Fatalf("Model(%v,0) error: %v", h, err)
		}
		if cur.Temperature != prev.Temperature {
			t.Errorf("temperature changed in isothermal layer at %v m: %.10f -> %.10f K",
				h, prev.Temperature, cur.Temperature)
		}
		if !(cur.Pressure < prev.Pressure) {
			t.Errorf("pressure did not decrease %v->%v m: %.8f -> %.8f Pa",
				h-step, h, prev.Pressure, cur.Pressure)
		}
		if !(cur.Density < prev.Density) {
			t.Errorf("density did not decrease %v->%v m: %.10f -> %.10f kg/m^3",
				h-step, h, prev.Density, cur.Density)
		}
		prev = cur
	}
}

// 声速只认温度：相同温度下（无论气压差多少）声速完全相同。
func TestSpeedOfSoundDependsOnlyOnTemperature(t *testing.T) {
	// 0 m and 11 km differ enormously in pressure; then equalize temperature
	// with an offset so the speed-of-sound inputs are identical.
	sea, err := Model(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	top, err := Model(TropopauseAltitude, sea.Temperature-topTemperature0())
	if err != nil {
		t.Fatal(err)
	}
	// Sanity: temperatures equal, pressures deliberately very different.
	if !approx(sea.Temperature, top.Temperature, absTol) {
		t.Fatalf("test setup: temperatures differ %.10f vs %.10f", sea.Temperature, top.Temperature)
	}
	if sea.Pressure < 3*top.Pressure {
		t.Fatalf("test setup: pressures should differ strongly, %.1f vs %.1f", sea.Pressure, top.Pressure)
	}
	if sea.SpeedOfSound != top.SpeedOfSound {
		t.Errorf("speed of sound changed with pressure at equal temperature: %.12f vs %.12f m/s",
			sea.SpeedOfSound, top.SpeedOfSound)
	}
}

// topTemperature0 is the standard tropopause temperature (test helper).
func topTemperature0() float64 { return tropopauseTemperature }

// 声速走绝热公式 a=sqrt(gamma*R*T)，与气压无关——直接对公式做数值核对。
func TestSpeedOfSoundFormula(t *testing.T) {
	sea, _ := Model(0, 0)
	want := math.Sqrt(HeatCapacityRatio * GasConstant * SeaLevelTemperature)
	if !approxRel(sea.SpeedOfSound, want, 1e-12) {
		t.Errorf("a(0) = %.10f, want %.10f m/s", sea.SpeedOfSound, want)
	}
}

// 无温度偏差时，正算密度与密度高度反算互为逆运算：必须精确回到输入高度。
func TestDensityAltitudeInverseRoundTrip(t *testing.T) {
	heights := []float64{0, 1, 1000, 5000, 10000, 10999.999, TropopauseAltitude, 11001, 15000, 19999.999, ModelCeiling}
	for _, h := range heights {
		a, err := Model(h, 0)
		if err != nil {
			t.Fatalf("Model(%v,0) error: %v", h, err)
		}
		inv, err := DensityAltitude(a.Density)
		if err != nil {
			t.Fatalf("DensityAltitude error at %v m: %v", h, err)
		}
		if !approx(inv, h, 1e-6) {
			t.Errorf("density altitude round-trip at %.3f m: got %.9f m, diff = %.3e m",
				h, inv, inv-h)
		}
	}
}

// 叠了温度偏差：标准气压廓线不变；暖空气密度变小、密度高度高于几何高度，
// 冷空气相反；且无偏差时结果与纯标准大气完全一致。
func TestTemperatureOffset(t *testing.T) {
	h := 8000.0
	std, err := Model(h, 0)
	if err != nil {
		t.Fatal(err)
	}
	warm, err := Model(h, +15)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := Model(h, -15)
	if err != nil {
		t.Fatal(err)
	}

	// Pressure profile is untouched by the offset.
	if warm.Pressure != std.Pressure || cold.Pressure != std.Pressure {
		t.Errorf("pressure changed with temperature offset: std=%.6f warm=%.6f cold=%.6f",
			std.Pressure, warm.Pressure, cold.Pressure)
	}
	// Temperatures shift by exactly the offset; standard temperature stays reported.
	if !approx(warm.Temperature, std.Temperature+15, absTol) {
		t.Errorf("warm temperature = %.8f, want %.8f", warm.Temperature, std.Temperature+15)
	}
	if !approx(cold.Temperature, std.Temperature-15, absTol) {
		t.Errorf("cold temperature = %.8f, want %.8f", cold.Temperature, std.Temperature-15)
	}
	if warm.StandardTemperature != std.StandardTemperature || cold.StandardTemperature != std.StandardTemperature {
		t.Errorf("standard temperature field must be independent of offset")
	}
	// Density recomputed with the actual temperature.
	if !(warm.Density < std.Density && std.Density < cold.Density) {
		t.Errorf("density ordering wrong: warm=%.8f std=%.8f cold=%.8f",
			warm.Density, std.Density, cold.Density)
	}
	// Density altitude departs from geometric altitude in the expected direction.
	if !(warm.DensityAltitude > h) {
		t.Errorf("warm density altitude %.3f should exceed geometric %.3f", warm.DensityAltitude, h)
	}
	if !(cold.DensityAltitude < h) {
		t.Errorf("cold density altitude %.3f should be below geometric %.3f", cold.DensityAltitude, h)
	}
	// Same test inside the isothermal layer.
	warm2, err := Model(15000, +15)
	if err != nil {
		t.Fatal(err)
	}
	if !(warm2.DensityAltitude > 15000) {
		t.Errorf("warm density altitude in isothermal layer %.3f should exceed 15000", warm2.DensityAltitude)
	}
}

// 非法高度：低于海平面、超过 20 km、非有限值，都返回带原因的错误，绝不外推。
func TestInvalidAltitudesRejected(t *testing.T) {
	bad := []float64{-0.001, -100, -1000, ModelCeiling + 0.001, 25000, 100000, math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, h := range bad {
		if _, err := Model(h, 0); err == nil {
			t.Errorf("Model(%v,0) expected error, got nil", h)
		} else if _, ok := err.(ValidationError); !ok {
			t.Errorf("Model(%v,0) expected ValidationError, got %T: %v", h, err, err)
		}
	}
}

// 批量剖面：取点、步长、边界和逐点模型结果一致。
func TestProfile(t *testing.T) {
	points, err := Profile(0, 10000, 2000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 6 {
		t.Fatalf("profile length = %d, want 6 (0..10000 step 2000)", len(points))
	}
	for i, p := range points {
		wantH := float64(i * 2000)
		if !approx(p.Altitude, wantH, absTol) {
			t.Errorf("point %d altitude = %v, want %v", i, p.Altitude, wantH)
		}
		direct, err := Model(wantH, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !approxRel(p.Pressure, direct.Pressure, relTol) || !approxRel(p.Density, direct.Density, relTol) {
			t.Errorf("point %d disagrees with direct Model call", i)
		}
	}

	// Endpoint not an exact multiple: it is not stretched.
	points, err = Profile(0, 5500, 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 6 || points[5].Altitude != 5000 {
		t.Errorf("non-multiple endpoint handling wrong: n=%d last=%v", len(points), points[len(points)-1].Altitude)
	}

	if _, err := Profile(-1, 1000, 100, 0); err == nil {
		t.Error("negative start should be rejected")
	}
	if _, err := Profile(0, 21000, 100, 0); err == nil {
		t.Error("end above ceiling should be rejected")
	}
	if _, err := Profile(0, 1000, 0, 0); err == nil {
		t.Error("zero step should be rejected")
	}
	if _, err := Profile(1000, 0, 100, 0); err == nil {
		t.Error("end < start should be rejected")
	}

	// Offset propagates through the whole profile.
	warm, err := Profile(0, 4000, 2000, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range warm {
		if !approx(p.Temperature-p.StandardTemperature, 10, absTol) {
			t.Errorf("offset not applied at %v m", p.Altitude)
		}
	}
}

// 万米出头巡航示范层（11 km 对流层顶）的标准值核对。
func TestCruiseDemoValuesAtTropopause(t *testing.T) {
	a, err := Model(TropopauseAltitude, 0)
	if err != nil {
		t.Fatal(err)
	}
	// T1 = 288.15 - 6.5*11 = 216.65 K
	if !approx(a.Temperature, 216.65, 1e-9) {
		t.Errorf("tropopause T = %.6f K, want 216.65 K", a.Temperature)
	}
	// Tabulated standard value ≈ 22632.06 Pa (tolerance covers tabulation rounding).
	if !approxRel(a.Pressure, 22632.06, 1e-4) {
		t.Errorf("tropopause P = %.6f Pa, want ~22632.06 Pa", a.Pressure)
	}
	if !approxRel(a.Density, 0.3639, 1e-3) {
		t.Errorf("tropopause rho = %.6f kg/m^3, want ~0.3639", a.Density)
	}
}
