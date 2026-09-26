package isa

import "math"

// Isothermal layer: TropopauseAltitude <= h <= ModelCeiling.
//
// Temperature is constant at the tropopause value T1, and pressure follows
// the isothermal-atosphere exponential law (integrating dP/dh = -rho*g with
// rho = P/(R*T1)):
//
//	P(h) = P1 * exp( -g*(h-h1)/(R*T1) ).
//
// P1/T1 below are exactly the values produced by the tropospheric formulas at
// h1, so there is no pressure jump at the boundary.

// isothermalTemperature returns the (constant) temperature of the layer.
func isothermalTemperature(_ float64) float64 {
	return tropopauseTemperature
}

// isothermalPressure returns the standard pressure at geometric altitude h
// (metres) within the isothermal layer.
func isothermalPressure(h float64) float64 {
	return tropopausePressure * math.Exp(-isothermalDecayCoefficient*(h-TropopauseAltitude))
}

// isothermalDensity returns the standard density at geometric altitude h
// (metres) within the isothermal layer.
func isothermalDensity(h float64) float64 {
	return idealGasDensity(isothermalPressure(h), tropopauseTemperature)
}
