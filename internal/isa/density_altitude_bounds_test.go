package isa

import (
	"math"
	"strings"
	"testing"
)

// 复现联调发现的缺陷：0 m 叠 -100 K 偏移（模拟极寒），空气密度比海平面
// 标准值还稠，反解出的密度高度约 -4670 m，落在模型分层范围之外。
// 必须识别为超出模型能力范围的输入并拒绝，不能把野值当正常结果返回。
func TestDensityAltitudeBelowSeaLevelRejected(t *testing.T) {
	a, err := Model(0, -100)
	if err == nil {
		t.Fatalf("Model(0,-100) expected out-of-range error, got density altitude %.3f m", a.DensityAltitude)
	}
	ve, ok := err.(ValidationError)
	if !ok {
		t.Fatalf("Model(0,-100) expected ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(string(ve), "density altitude") {
		t.Errorf("reason should identify the density altitude as out of range, got: %v", ve)
	}

	// 直接对反解函数验证：密度高于海平面标准值 → 反解高度低于 0 m → 拒绝。
	dense := idealGasDensity(SeaLevelPressure, SeaLevelTemperature-100)
	if dense <= seaLevelDensityComputed {
		t.Fatalf("test setup: density %.6f should exceed sea-level standard %.6f", dense, seaLevelDensityComputed)
	}
	if h, err := DensityAltitude(dense); err == nil {
		t.Errorf("DensityAltitude(%.6f) expected error for sub-sea-level inversion, got %.3f m", dense, h)
	} else if _, ok := err.(ValidationError); !ok {
		t.Errorf("DensityAltitude(%.6f) expected ValidationError, got %T: %v", dense, err, err)
	}
}

// 反方向：极热空气密度比 20 km 模型顶层还稀，反解高度超出模型上限，
// 同样必须拒绝而不是外推一个无物理意义的数字。
func TestDensityAltitudeAboveCeilingRejected(t *testing.T) {
	a, err := Model(ModelCeiling, 1000)
	if err == nil {
		t.Fatalf("Model(20000,1000) expected out-of-range error, got density altitude %.3f m", a.DensityAltitude)
	}
	if _, ok := err.(ValidationError); !ok {
		t.Fatalf("Model(20000,1000) expected ValidationError, got %T: %v", err, err)
	}

	// 直接对反解函数验证：密度低于 20 km 标准值 → 反解高度超出上限 → 拒绝。
	thin := idealGasDensity(isothermalPressure(ModelCeiling), tropopauseTemperature+1000)
	ceilingDensity := idealGasDensity(isothermalPressure(ModelCeiling), tropopauseTemperature)
	if thin >= ceilingDensity {
		t.Fatalf("test setup: density %.6f should be below ceiling standard %.6f", thin, ceilingDensity)
	}
	if h, err := DensityAltitude(thin); err == nil {
		t.Errorf("DensityAltitude(%.6f) expected error for above-ceiling inversion, got %.3f m", thin, h)
	} else if _, ok := err.(ValidationError); !ok {
		t.Errorf("DensityAltitude(%.6f) expected ValidationError, got %T: %v", thin, err, err)
	}
}

// 边界本身不被误伤：h=0 与 h=20000 的标准大气反解必须落在边界上并正常返回
// （浮点回环噪声由容差吸收，实测 < 2e-12 m）。
func TestDensityAltitudeBoundariesAccepted(t *testing.T) {
	sea, err := Model(0, 0)
	if err != nil {
		t.Fatalf("Model(0,0) error: %v", err)
	}
	if !approx(sea.DensityAltitude, 0, 1e-9) {
		t.Errorf("sea-level density altitude = %.10f, want 0", sea.DensityAltitude)
	}
	top, err := Model(ModelCeiling, 0)
	if err != nil {
		t.Fatalf("Model(20000,0) error: %v", err)
	}
	if !approx(top.DensityAltitude, ModelCeiling, 1e-6) {
		t.Errorf("ceiling density altitude = %.10f, want 20000", top.DensityAltitude)
	}

	// 直接反解两个边界的标准密度也不能报错。
	if h, err := DensityAltitude(seaLevelDensityComputed); err != nil {
		t.Errorf("DensityAltitude(sea-level density) error: %v", err)
	} else if !approx(h, 0, 1e-9) {
		t.Errorf("sea-level density inversion = %.10f, want 0", h)
	}
	ceilingDensity := idealGasDensity(isothermalPressure(ModelCeiling), tropopauseTemperature)
	if h, err := DensityAltitude(ceilingDensity); err != nil {
		t.Errorf("DensityAltitude(ceiling density) error: %v", err)
	} else if !approx(h, ModelCeiling, 1e-6) {
		t.Errorf("ceiling density inversion = %.9f, want 20000", h)
	}
}

// 防误伤：只要反解高度落在 [0, 20000] 内，不管偏移多大、密度高度离几何
// 高度多远，都必须正常返回。
func TestInRangeOffsetScenariosUnaffected(t *testing.T) {
	cases := []struct {
		h, deltaT float64
	}{
		{8000, -15},   // 冷空气：密度高度 ~7435 m，明显低于几何高度但在范围内
		{5000, -30},   // 更冷：密度高度 ~3829 m
		{11000, -100}, // 对流层顶极冷：密度高度 ~5781 m
		{1000, 50},    // 暖空气：密度高度 ~2632 m，高于几何高度
		{15000, 15},   // 等温层偏暖：密度高度 ~15425 m
		{0, 100},      // 海平面极热但反解 ~2997 m，仍在范围内
		{20000, -50},  // 顶层偏冷：反解 ~18336 m，仍在范围内
	}
	for _, tc := range cases {
		a, err := Model(tc.h, tc.deltaT)
		if err != nil {
			t.Errorf("Model(%v,%v) unexpectedly rejected: %v", tc.h, tc.deltaT, err)
			continue
		}
		if a.DensityAltitude < 0 || a.DensityAltitude > ModelCeiling {
			t.Errorf("Model(%v,%v) density altitude %.3f outside [0, 20000] yet returned success",
				tc.h, tc.deltaT, a.DensityAltitude)
		}
	}
}

// 批量剖面（冷侧）：越界点单独判废、带高度和原因，其余合法点照常返回；
// 严格版 Profile 保持原契约，任一采样点失败即整体报错。
func TestProfilePointsPartialFailureCold(t *testing.T) {
	// delta_t=-100 时，0/2000/4000 m 的密度高度约 -4670/-2740/-819 m（越界），
	// 6000/8000/10000 m 约 1089/2981/4854 m（范围内）。
	results, err := ProfilePoints(0, 10000, 2000, -100)
	if err != nil {
		t.Fatalf("ProfilePoints structural error: %v", err)
	}
	if len(results) != 6 {
		t.Fatalf("got %d points, want 6", len(results))
	}

	for i, h := range []float64{0, 2000, 4000} {
		r := results[i]
		if r.Err == nil {
			t.Errorf("point at %v m should have failed (density altitude below sea level)", h)
			continue
		}
		if r.Altitude != h {
			t.Errorf("failed point altitude = %v, want %v", r.Altitude, h)
		}
		if _, ok := r.Err.(ValidationError); !ok {
			t.Errorf("point at %v m: expected ValidationError, got %T: %v", h, r.Err, r.Err)
		}
	}
	for i, h := range []float64{6000, 8000, 10000} {
		r := results[i+3]
		if r.Err != nil {
			t.Errorf("point at %v m should succeed, got: %v", h, r.Err)
			continue
		}
		if !approx(r.Atmosphere.Altitude, h, absTol) {
			t.Errorf("point altitude = %v, want %v", r.Atmosphere.Altitude, h)
		}
		if r.Atmosphere.DensityAltitude < 0 || r.Atmosphere.DensityAltitude > ModelCeiling {
			t.Errorf("point at %v m: density altitude %.3f outside modelled range",
				h, r.Atmosphere.DensityAltitude)
		}
	}

	if _, err := Profile(0, 10000, 2000, -100); err == nil {
		t.Error("strict Profile should abort on the first out-of-range sample")
	}
}

// 批量剖面（热侧）：delta_t=+1000 时 10000/15000/20000 m 的反解高度约
// 20991/25943/30943 m（越界），0/5000 m 约 12799/16626 m（范围内）。
func TestProfilePointsPartialFailureHot(t *testing.T) {
	results, err := ProfilePoints(0, 20000, 5000, 1000)
	if err != nil {
		t.Fatalf("ProfilePoints structural error: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("got %d points, want 5", len(results))
	}

	for i, h := range []float64{0, 5000} {
		if r := results[i]; r.Err != nil {
			t.Errorf("point at %v m should succeed, got: %v", h, r.Err)
		} else if r.Atmosphere.DensityAltitude < 0 || r.Atmosphere.DensityAltitude > ModelCeiling {
			t.Errorf("point at %v m: density altitude %.3f outside modelled range",
				h, r.Atmosphere.DensityAltitude)
		}
	}
	for i, h := range []float64{10000, 15000, 20000} {
		r := results[i+2]
		if r.Err == nil {
			t.Errorf("point at %v m should have failed (density altitude above ceiling)", h)
			continue
		}
		if r.Altitude != h {
			t.Errorf("failed point altitude = %v, want %v", r.Altitude, h)
		}
		if _, ok := r.Err.(ValidationError); !ok {
			t.Errorf("point at %v m: expected ValidationError, got %T: %v", h, r.Err, r.Err)
		}
	}
}

// ProfilePoints 的结构性参数校验与严格版一致：非法区间整体报错。
func TestProfilePointsStructuralValidation(t *testing.T) {
	for _, args := range [][4]float64{
		{-1, 1000, 100, 0},         // start below sea level
		{0, 21000, 100, 0},         // end above ceiling
		{0, 1000, 0, 0},            // zero step
		{1000, 0, 100, 0},          // end < start
		{0, 100000, 0.5, 0},        // too many points
		{0, 1000, 100, math.NaN()}, // non-finite offset
	} {
		if _, err := ProfilePoints(args[0], args[1], args[2], args[3]); err == nil {
			t.Errorf("ProfilePoints%v expected structural error, got nil", args)
		}
	}
}
