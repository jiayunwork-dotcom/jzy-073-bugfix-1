package isa

import "math"

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
// the dry-air density equals rho (kg/m^3). A non-positive density is invalid.
//
// The inversion may legitimately fall below sea level (dense cold air) or
// above the 20 km ceiling (very thin hot air); those are mathematical results
// within the standard layer formulas, not invalid inputs.
func DensityAltitude(rho float64) (float64, error) {
	if math.IsNaN(rho) || math.IsInf(rho, 0) || rho <= 0 {
		return 0, InvalidArgumentError("density must be a finite positive value")
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
