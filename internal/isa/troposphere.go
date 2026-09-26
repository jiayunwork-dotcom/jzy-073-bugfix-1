package isa

import "math"

// Troposphere: 0 m <= h <= TropopauseAltitude.
//
// Temperature falls linearly: T(h) = T0 - L*h.
// Pressure follows the hydrostatic barometric formula:
//
//	P(h) = P0 * (1 - L*h/T0)^(g/(R*L)).

// troposphereTemperature returns the standard temperature at geometric
// altitude h (metres) within the troposphere.
func troposphereTemperature(h float64) float64 {
	return SeaLevelTemperature - LapseRate*h
}

// tropospherePressure returns the standard pressure at geometric altitude h
// (metres) within the troposphere.
func tropospherePressure(h float64) float64 {
	ratio := 1.0 - LapseRate*h/SeaLevelTemperature
	return SeaLevelPressure * math.Pow(ratio, pressureExponent)
}

// troposphereDensity returns the standard density at geometric altitude h
// (metres) within the troposphere.
func troposphereDensity(h float64) float64 {
	return idealGasDensity(tropospherePressure(h), troposphereTemperature(h))
}
