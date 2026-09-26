package isa

import (
	"fmt"
	"math"
)

// Density altitude is the altitude in the standard atmosphere at which the
// standard air density equals the supplied (possibly non-standard) density.
//
// The inverse therefore always runs through the *standard* temperature and
// pressure profiles: the current temperature offset never enters this
// function. Inverting a point computed without an offset reproduces the input
// geometric altitude exactly; with an offset, the result diverges from the
// geometric altitude — that divergence is the physical meaning of density
// altitude.

// densityAltitudeTolerance absorbs floating-point round-trip noise at the
// edges of the modelled range: inverting the density produced by the forward
// model at exactly h = 0 or h = ModelCeiling can land a few nanometres
// outside the interval (measured round-trip error is below 2e-12 m). Any
// genuinely out-of-range input overshoots the boundary by orders of
// magnitude more than this tolerance.
const densityAltitudeTolerance = 1e-6 // m

// DensityAltitude returns the standard-atmosphere altitude (metres) at which
// the dry-air density equals rho (kg/m^3). A non-positive density is invalid.
//
// The inversion is only defined inside the layers the model actually covers
// (0 m to ModelCeiling). A result below sea level (air denser than the
// sea-level standard, e.g. extreme cold) or above the 20 km ceiling (air
// thinner than the top of the isothermal layer, e.g. extreme heat) would be
// an extrapolation beyond the implemented formulas, so it is rejected with a
// ValidationError instead of returning a physically meaningless number.
func DensityAltitude(rho float64) (float64, error) {
	if math.IsNaN(rho) || math.IsInf(rho, 0) || rho <= 0 {
		return 0, InvalidArgumentError("density must be a finite positive value")
	}

	var h float64
	// Branch on the standard tropopause density, matching the forward
	// dispatch at exactly h1 (which belongs to the tropospheric branch).
	if rho >= tropopauseDensity {
		// Troposphere:
		//   rho/rho0 = (1 - L*h/T0)^(n-1)
		// with n = g/(R*L).
		ratio := math.Pow(rho/seaLevelDensityComputed, 1.0/(pressureExponent-1.0))
		h = SeaLevelTemperature / LapseRate * (1.0 - ratio)
	} else {
		// Isothermal layer:
		//   rho/rho1 = exp( -g*(h-h1)/(R*T1) )
		delta := -math.Log(rho/tropopauseDensity) / isothermalDecayCoefficient
		h = TropopauseAltitude + delta
	}

	// Refuse to extrapolate beyond the layers the model implements.
	switch {
	case h < -densityAltitudeTolerance:
		return 0, InvalidArgumentError(fmt.Sprintf(
			"density altitude %.3f m falls below the modelled layers (0-20000 m): density %.6g kg/m^3 is higher than the sea-level standard; no extrapolation is performed",
			h, rho))
	case h > ModelCeiling+densityAltitudeTolerance:
		return 0, InvalidArgumentError(fmt.Sprintf(
			"density altitude %.3f m exceeds the modelled layers (0-20000 m): density %.6g kg/m^3 is lower than the 20 km ceiling standard; no extrapolation is performed",
			h, rho))
	}
	return h, nil
}
