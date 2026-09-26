// Package isa implements the International Standard Atmosphere (ISA) model
// from sea level up to the 20 km implementation ceiling.
//
// Layers:
//   - Troposphere: 0 m <= h <= 11000 m, temperature decreases linearly with a
//     constant lapse rate of 6.5 K/km.
//   - Isothermal layer (lower stratosphere): 11000 m <= h <= 20000 m, the
//     temperature is pinned at the tropopause value and pressure decays
//     exponentially with altitude.
//
// All reference/physical constants are pinned in this file and are the single
// source of truth shared by the tropospheric, isothermal and density-altitude
// formulas; callers cannot override them.
package isa

import "math"

// Pinned International Standard Atmosphere reference values.
const (
	// Sea-level reference conditions (geometric altitude h = 0 m).
	SeaLevelTemperature = 288.15   // K (15 °C)
	SeaLevelPressure    = 101325.0 // Pa
	SeaLevelDensity     = 1.225    // kg/m^3

	// Tropospheric temperature lapse rate: 6.5 K per km.
	LapseRate = 0.0065 // K/m

	// Layer boundaries (geometric altitude, metres).
	TropopauseAltitude = 11000.0 // tropopause: top of troposphere
	ModelCeiling       = 20000.0 // implementation limit: top of isothermal layer

	// Physical constants.
	Gravity           = 9.80665 // m/s^2, standard gravitational acceleration
	HeatCapacityRatio = 1.4     // gamma, adiabatic index of dry air

	// Specific gas constant for dry air, J/(kg·K). Pinned by the ISA sea-level
	// reference values (numerically the tabulated 287.05287 J/(kg·K)); defining
	// it from the same constants guarantees rho(0) reproduces SeaLevelDensity
	// exactly and that forward/inverse formulas never drift apart.
	GasConstant = SeaLevelPressure / (SeaLevelDensity * SeaLevelTemperature)
)

// Derived quantities, evaluated once from the constants above so that the
// tropospheric and isothermal layers share exactly the same tropopause state.
var (
	// n = g/(R*L): exponent of the tropospheric barometric formula.
	pressureExponent = Gravity / (GasConstant * LapseRate)

	// Tropopause temperature T1 = T0 - L*h1. It is evaluated through the
	// tropospheric function (runtime float64 arithmetic, not compile-time
	// constant folding) so that T(h1) from either layer formula is bit for bit
	// identical — that is what makes the layer boundary genuinely continuous.
	tropopauseTemperature = troposphereTemperature(TropopauseAltitude)

	// Tropopause pressure P1, evaluated with the tropospheric formula at h1.
	// The isothermal formula starts from this same value, which makes the
	// pressure profile continuous at the layer boundary by construction.
	tropopausePressure = tropospherePressure(TropopauseAltitude)

	// Tropopause density under the standard atmosphere.
	tropopauseDensity = idealGasDensity(tropopausePressure, tropopauseTemperature)

	// Density at h = 0 produced by the ideal-gas equation from the pinned
	// P0/T0 (differs from 1.225 by at most floating-point rounding). Used as
	// the reference for density-altitude inversion so the two are exact
	// inverses.
	seaLevelDensityComputed = idealGasDensity(SeaLevelPressure, SeaLevelTemperature)

	// g/(R*T1): pressure/density decay coefficient in the isothermal layer.
	isothermalDecayCoefficient = Gravity / (GasConstant * tropopauseTemperature)
)

// idealGasDensity returns the air density from the ideal-gas state equation
// rho = P/(R*T). Shared by every layer and by every temperature scenario.
func idealGasDensity(pressure, temperature float64) float64 {
	return pressure / (GasConstant * temperature)
}

// speedOfSound returns the adiabatic speed of sound a = sqrt(gamma*R*T).
// It depends on temperature only; pressure does not enter the formula.
func speedOfSound(temperature float64) float64 {
	return math.Sqrt(HeatCapacityRatio * GasConstant * temperature)
}
