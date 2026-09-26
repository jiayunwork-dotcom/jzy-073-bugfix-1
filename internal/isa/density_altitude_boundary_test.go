package isa

import (
	"math"
	"testing"
)

// assertOutOfRange asserts that err is an *OutOfModelRangeError on the given
// side, with the diagnostic equivalent altitude matching the inversion.
func assertOutOfRange(t *testing.T, err error, direction string) *OutOfModelRangeError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected out-of-model-range error (%s), got nil", direction)
	}
	r, ok := AsOutOfModelRangeError(err)
	if !ok {
		t.Fatalf("expected *OutOfModelRangeError, got %T: %v", err, err)
	}
	if r.Direction != direction {
		t.Fatalf("direction = %q, want %q (err: %v)", r.Direction, direction, err)
	}
	if !math.IsNaN(r.DensityAltitude) && !math.IsInf(r.DensityAltitude, 0) {
		switch direction {
		case RangeBelowFloor:
			if r.DensityAltitude >= 0 {
				t.Errorf("below-floor diagnostic altitude = %v, want < 0", r.DensityAltitude)
			}
		case RangeAboveCeiling:
			if r.DensityAltitude <= ModelCeiling {
				t.Errorf("above-ceiling diagnostic altitude = %v, want > %v", r.DensityAltitude, ModelCeiling)
			}
		}
	}
	return r
}

// 极冷（深负温度偏差）使空气比海平面标准大气还稠密，密度高度反解落到
// 0 m 以下：Model 必须以结构化的越界错误拒绝，而不是返回负几千米的野值。
func TestModelRejectsDensityAltitudeBelowSeaLevel(t *testing.T) {
	// The reported case: sea level with a -100 K offset.
	_, err := Model(0, -100)
	r := assertOutOfRange(t, err, RangeBelowFloor)
	if r.DensityAltitude > -1000 {
		t.Errorf("equivalent altitude for h=0,dT=-100 = %.2f m, want a clearly negative value (thousands of metres)",
			r.DensityAltitude)
	}
	t.Logf("h=0, delta_t=-100: rejected below-floor inversion, equivalent altitude = %.2f m", r.DensityAltitude)

	// The same condition at a non-sea-level geometric altitude.
	_, err = Model(1000, -100)
	assertOutOfRange(t, err, RangeBelowFloor)

	// Direct on the inverse function with a density above sea level.
	_, err = DensityAltitude(seaLevelDensityComputed * 1.05)
	assertOutOfRange(t, err, RangeBelowFloor)
}

// 极热（深正温度偏差）使空气比 20 km 处标准大气还稀薄，密度高度反解落到
// 20 km 以上：Model 必须以结构化的越界错误拒绝，而不是外推一个高度。
func TestModelRejectsDensityAltitudeAboveCeiling(t *testing.T) {
	// At the ceiling, a +50 K offset pushes the equivalent altitude above 20 km.
	_, err := Model(ModelCeiling, 50)
	r := assertOutOfRange(t, err, RangeAboveCeiling)
	if r.DensityAltitude < ModelCeiling+100 {
		t.Errorf("equivalent altitude for h=20000,dT=+50 = %.2f m, want clearly above ceiling",
			r.DensityAltitude)
	}
	t.Logf("h=20000, delta_t=+50: rejected above-ceiling inversion, equivalent altitude = %.2f m",
		r.DensityAltitude)

	// Direct on the inverse function with a density below the ceiling density.
	_, err = DensityAltitude(ceilingDensity * 0.95)
	assertOutOfRange(t, err, RangeAboveCeiling)
}

// 反解恰好落在 0 m / 20000 m 边界密度上时必须被接受（边界本身属于模型），
// 且分别精确回到 0 m 与 20000 m——不能因为浮点噪声把合法边界点误杀。
func TestDensityAltitudeBoundariesAreInclusive(t *testing.T) {
	if h, err := DensityAltitude(seaLevelDensityComputed); err != nil {
		t.Errorf("density exactly at sea-level value rejected: %v", err)
	} else if !approx(h, 0, 1e-9) {
		t.Errorf("inversion at sea-level density = %.12f m, want 0", h)
	}
	if h, err := DensityAltitude(ceilingDensity); err != nil {
		t.Errorf("density exactly at ceiling value rejected: %v", err)
	} else if !approx(h, ModelCeiling, 1e-6) {
		t.Errorf("inversion at ceiling density = %.9f m, want %v", h, ModelCeiling)
	}

	// And via the full model: standard sea level and standard ceiling points
	// must remain accepted (no offset).
	if _, err := Model(0, 0); err != nil {
		t.Errorf("Model(0,0) rejected: %v", err)
	}
	if _, err := Model(ModelCeiling, 0); err != nil {
		t.Errorf("Model(20000,0) rejected: %v", err)
	}
}

// 合法但密度高度与几何高度相差很远的请求绝不能被边界检查误伤：
// 一个深偏差只要等效高度仍在 [0, 20000] 内，就照常返回结果。
func TestExtremeButInRangeOffsetsStillCompute(t *testing.T) {
	cases := []struct {
		h, dT float64
	}{
		{10000, -100}, // very cold aloft: equivalent altitude ~6.2 km, far below geometric but in range
		{0, 250},      // hot at sea level: equivalent altitude well under the ceiling
		{20000, -60},  // very cold at the ceiling: equivalent altitude still positive
	}
	for _, c := range cases {
		a, err := Model(c.h, c.dT)
		if err != nil {
			t.Errorf("Model(%v,%v) unexpectedly rejected: %v", c.h, c.dT, err)
			continue
		}
		if a.DensityAltitude < 0 || a.DensityAltitude > ModelCeiling {
			t.Errorf("Model(%v,%v) density altitude = %.3f m, want within [0,%v]",
				c.h, c.dT, a.DensityAltitude, ModelCeiling)
		}
	}
}

// 批量剖面：采样点的温度/气压等非密度高度量与逐点 Model 一致（字段嵌入保持
// 原有访问方式与行为）。
func TestProfilePointsEmbedAtmosphere(t *testing.T) {
	points, err := Profile(0, 6000, 2000, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range points {
		if p.Err != nil {
			t.Fatalf("point %d unexpectedly failed: %v", i, p.Err)
		}
		if p.Index != i {
			t.Errorf("point %d index = %d", i, p.Index)
		}
		wantH := float64(i * 2000)
		direct, err := Model(wantH, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !approxRel(p.Pressure, direct.Pressure, relTol) ||
			!approxRel(p.Density, direct.Density, relTol) ||
			!approx(p.DensityAltitude, wantH, 1e-6) {
			t.Errorf("point %d (%v m) disagrees with direct Model call", i, p.Altitude)
		}
	}
}

// 批量剖面，偏冷方向：只有越界的那一个采样点失败并带结构化原因，
// 其余点全部正常——不能因一个点报废整批，也不能混过野值。
func TestProfilePerPointFailureColdSide(t *testing.T) {
	// step 10000 m: 0 m (below-floor with -100 K), 10000 m and 20000 m fine.
	points, err := Profile(0, ModelCeiling, 10000, -100)
	if err != nil {
		t.Fatalf("whole-batch error not expected: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("profile length = %d, want 3", len(points))
	}
	failed := 0
	for _, p := range points {
		switch p.Index {
		case 0:
			if p.Err == nil {
				t.Errorf("point 0 (0 m) should fail below-floor, got density altitude %v", p.DensityAltitude)
			} else {
				assertOutOfRange(t, p.Err, RangeBelowFloor)
				failed++
			}
		case 1, 2:
			if p.Err != nil {
				t.Errorf("point %d (%v m) should succeed, got: %v", p.Index, p.Altitude, p.Err)
				continue
			}
			direct, err := Model(p.Altitude, -100)
			if err != nil {
				t.Errorf("point %d (%v m) direct Model disagrees: %v", p.Index, p.Altitude, err)
				continue
			}
			if !approxRel(p.Pressure, direct.Pressure, relTol) ||
				!approx(p.DensityAltitude, direct.DensityAltitude, 1e-6) {
				t.Errorf("point %d values disagree with direct Model call", p.Index)
			}
		}
	}
	if failed != 1 {
		t.Errorf("failed point count = %d, want exactly 1", failed)
	}
}

// 批量剖面，偏热方向：只有 20 km 那个采样点越界失败，其余正常。
func TestProfilePerPointFailureHotSide(t *testing.T) {
	points, err := Profile(0, ModelCeiling, 10000, 50)
	if err != nil {
		t.Fatalf("whole-batch error not expected: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("profile length = %d, want 3", len(points))
	}
	failed := 0
	for _, p := range points {
		if p.Index == 2 {
			if p.Err == nil {
				t.Errorf("point 2 (20000 m) should fail above-ceiling, got density altitude %v", p.DensityAltitude)
			} else {
				assertOutOfRange(t, p.Err, RangeAboveCeiling)
				failed++
			}
			continue
		}
		if p.Err != nil {
			t.Errorf("point %d (%v m) should succeed, got: %v", p.Index, p.Altitude, p.Err)
		}
	}
	if failed != 1 {
		t.Fatalf("failed point count = %d, want exactly 1", failed)
	}

	// Every sampled position is still represented (the bad point did not
	// delete the slot or shift indices).
	if points[2].Altitude != ModelCeiling || points[2].Index != 2 {
		t.Errorf("failed slot position lost: %+v", points[2])
	}
}

// 结构性非法的批量请求仍整批拒绝（与逐点越界区分）。
func TestProfileStructuralErrorsStillRejectWholeBatch(t *testing.T) {
	if _, err := Profile(-1, 1000, 100, 0); err == nil {
		t.Error("negative start should still reject the whole batch")
	}
	if _, err := Profile(0, 21000, 100, 0); err == nil {
		t.Error("end above ceiling should still reject the whole batch")
	}
	if _, err := Profile(0, 1000, 1, math.NaN()); err == nil {
		t.Error("non-finite offset should still reject the whole batch")
	}
}
