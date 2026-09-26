package isa

import (
	"errors"
	"math"
	"strconv"
)

// Direction of an out-of-model-range density-altitude inversion.
const (
	// RangeBelowFloor means the air is denser than standard sea-level air, so
	// the equivalent altitude lies below 0 m.
	RangeBelowFloor = "below_sea_level"
	// RangeAboveCeiling means the air is thinner than the standard atmosphere
	// at ModelCeiling, so the equivalent altitude lies above 20000 m.
	RangeAboveCeiling = "above_ceiling"
)

// OutOfModelRangeError reports that a density-altitude inversion landed
// outside the altitude span covered by the implemented layer formulas
// ([0, ModelCeiling]). The service never presents such an equivalent altitude
// as a normal result: it has no physical meaning inside this model and would
// amount to silently extrapolating layers that are not implemented.
//
// DensityAltitude is the equivalent altitude the inversion mathematically
// produced; Direction tells which side of the model range it fell on.
type OutOfModelRangeError struct {
	// DensityAltitude is the raw equivalent altitude [m] produced by the
	// inversion; it is diagnostic only and is never returned as a result.
	DensityAltitude float64
	// Direction is one of RangeBelowFloor / RangeAboveCeiling.
	Direction string
}

func (e *OutOfModelRangeError) Error() string {
	switch e.Direction {
	case RangeBelowFloor:
		return "density altitude inversion fell below the 0 m model floor (equivalent altitude " +
			trimAltitude(e.DensityAltitude) + " m): the air is denser than standard sea-level air; " +
			"the implemented layers (0 m - 20000 m) do not cover such dense air, so no density altitude is returned"
	case RangeAboveCeiling:
		return "density altitude inversion rose above the 20000 m model ceiling (equivalent altitude " +
			trimAltitude(e.DensityAltitude) + " m): the air is thinner than the standard atmosphere at 20000 m; " +
			"the implemented layers (0 m - 20000 m) do not cover such thin air, so no density altitude is returned"
	default:
		return "density altitude inversion fell outside the implemented model range (0 m - 20000 m)"
	}
}

// AsOutOfModelRangeError unwraps err to *OutOfModelRangeError, if that is its
// type.
func AsOutOfModelRangeError(err error) (*OutOfModelRangeError, bool) {
	var target *OutOfModelRangeError
	return target, errors.As(err, &target)
}

// trimAltitude formats an equivalent altitude for diagnostic messages.
func trimAltitude(h float64) string {
	return strconv.FormatFloat(h, 'f', 1, 64)
}

// Density altitude is the altitude in the standard atmosphere at which the
// standard air density equals the supplied (possibly non-standard) density.
//
// The inverse therefore always runs through the *standard* temperature and
// pressure profiles: the current temperature offset never enters this
// function. Inverting a point computed without an offset reproduces the input
// geometric altitude exactly; with an offset, the result diverges from the
// geometric altitude — that divergence is the physical meaning of density
// altitude.

// DensityAltitude returns the standard-atmosphere altitude (metres) at which
// the dry-air density equals rho (kg/m^3). A non-positive or non-finite
// density is invalid.
//
// The equivalent altitude MUST stay within the altitude span of the
// implemented layers, i.e. within [0, ModelCeiling]. If rho is denser than
// standard sea-level air (equivalent altitude below 0 m) or thinner than the
// standard atmosphere at ModelCeiling (equivalent altitude above 20000 m),
// an *OutOfModelRangeError is returned: the model does not implement the
// layers such an altitude would live in, and extrapolating them would produce
// a number with no physical meaning.
func DensityAltitude(rho float64) (float64, error) {
	if math.IsNaN(rho) || math.IsInf(rho, 0) || rho <= 0 {
		return 0, InvalidArgumentError("density must be a finite positive value")
	}

	// Range guard on the density itself, compared against the densities at
	// the two implemented boundaries. Comparing densities (rather than the
	// inverted altitude) makes rho exactly at a boundary — produced by the
	// very same formulas in Model(0,0) / Model(ModelCeiling,0) — an exact
	// equality, so legal boundary points are never rejected on float noise.
	if rho > seaLevelDensityComputed {
		// Denser than sea level: the tropospheric inversion would land at a
		// negative equivalent altitude.
		ratio := math.Pow(rho/seaLevelDensityComputed, 1.0/(pressureExponent-1.0))
		h := SeaLevelTemperature / LapseRate * (1.0 - ratio)
		return 0, &OutOfModelRangeError{DensityAltitude: h, Direction: RangeBelowFloor}
	}
	if rho < ceilingDensity {
		// Thinner than at the 20 km ceiling: the isothermal inversion would
		// land above the ceiling.
		delta := -math.Log(rho/tropopauseDensity) / isothermalDecayCoefficient
		h := TropopauseAltitude + delta
		return 0, &OutOfModelRangeError{DensityAltitude: h, Direction: RangeAboveCeiling}
	}

	// Branch on the standard tropopause density, matching the forward
	// dispatch at exactly h1 (which belongs to the tropospheric branch).
	if rho >= tropopauseDensity {
		// Troposphere:
		//   rho/rho0 = (1 - L*h/T0)^(n-1)
		// with n = g/(R*L).
		ratio := math.Pow(rho/seaLevelDensityComputed, 1.0/(pressureExponent-1.0))
		return SeaLevelTemperature / LapseRate * (1.0 - ratio), nil
	}

	// Isothermal layer:
	//   rho/rho1 = exp( -g*(h-h1)/(R*T1) )
	delta := -math.Log(rho/tropopauseDensity) / isothermalDecayCoefficient
	return TropopauseAltitude + delta, nil
}
