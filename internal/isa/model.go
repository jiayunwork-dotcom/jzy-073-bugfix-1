package isa

import "math"

// ValidationError is returned for inputs the model refuses to evaluate
// (altitude below sea level, above the 20 km ceiling, non-finite values, ...).
// The message is a structured, human-readable reason.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// InvalidArgumentError builds a ValidationError with the given reason.
func InvalidArgumentError(reason string) error { return ValidationError(reason) }

// Atmosphere is the complete state of the atmosphere at one geometric
// altitude. Quantities use SI units.
type Atmosphere struct {
	// Altitude is the geometric altitude above mean sea level [m].
	Altitude float64 `json:"altitude_m"`
	// TemperatureOffset is the user-supplied delta-T applied to the standard
	// temperature [K] (0 for a pure standard atmosphere).
	TemperatureOffset float64 `json:"temperature_offset_k"`
	// StandardTemperature is T_std(h) before applying the offset [K].
	StandardTemperature float64 `json:"standard_temperature_k"`
	// Temperature is the (possibly offset) temperature actually used for
	// density and speed of sound [K].
	Temperature float64 `json:"temperature_k"`
	// Pressure always follows the standard altitude formula; the offset does
	// not change it [Pa].
	Pressure float64 `json:"pressure_pa"`
	// Density derives from pressure and the actual temperature via the
	// ideal-gas equation [kg/m^3].
	Density float64 `json:"density_kg_m3"`
	// SpeedOfSound follows the adiabatic formula and depends on the actual
	// temperature only [m/s].
	SpeedOfSound float64 `json:"speed_of_sound_m_s"`
	// DensityAltitude is the inverse solution in the standard atmosphere [m].
	DensityAltitude float64 `json:"density_altitude_m"`
}

// validateAltitude rejects everything outside the implemented model, so the
// service never extrapolates into layers it does not model.
func validateAltitude(h float64) error {
	switch {
	case math.IsNaN(h) || math.IsInf(h, 0):
		return InvalidArgumentError("altitude must be a finite number")
	case h < 0:
		return InvalidArgumentError("altitude below sea level is not supported (h < 0 m)")
	case h > ModelCeiling:
		return InvalidArgumentError("altitude above the 20 km implementation ceiling is not supported (h > 20000 m); no extrapolation is performed")
	default:
		return nil
	}
}

// standardState returns the standard temperature and pressure at h using the
// layer the altitude belongs to. The tropopause altitude itself belongs to
// the tropospheric branch; the isothermal branch is anchored on the same
// T1/P1, so both branches agree there.
func standardState(h float64) (temperature, pressure float64) {
	if h <= TropopauseAltitude {
		return troposphereTemperature(h), tropospherePressure(h)
	}
	return isothermalTemperature(h), isothermalPressure(h)
}

// Model evaluates the atmosphere at geometric altitude h (metres) with an
// optional uniform temperature offset deltaT (kelvin, 0 for ISA).
//
// Pressure is always taken from the standard altitude profile. The offset
// only replaces the temperature that feeds the ideal-gas density and the
// speed of sound — the standard pressure profile and the actual temperature
// are deliberately kept independent.
//
// An offset that pushes the density outside the modelled layers (density
// altitude below 0 m or above the 20 km ceiling) is rejected with a
// ValidationError: the model does not extrapolate.
func Model(h, deltaT float64) (Atmosphere, error) {
	if err := validateAltitude(h); err != nil {
		return Atmosphere{}, err
	}
	if math.IsNaN(deltaT) || math.IsInf(deltaT, 0) {
		return Atmosphere{}, InvalidArgumentError("temperature offset must be a finite number")
	}

	stdT, pressure := standardState(h)
	temperature := stdT + deltaT
	if temperature <= 0 {
		return Atmosphere{}, InvalidArgumentError("standard temperature plus offset must stay positive (in kelvin)")
	}

	density := idealGasDensity(pressure, temperature)
	densityAltitude, err := DensityAltitude(density)
	if err != nil {
		return Atmosphere{}, err
	}

	return Atmosphere{
		Altitude:            h,
		TemperatureOffset:   deltaT,
		StandardTemperature: stdT,
		Temperature:         temperature,
		Pressure:            pressure,
		Density:             density,
		SpeedOfSound:        speedOfSound(temperature),
		DensityAltitude:     densityAltitude,
	}, nil
}

// MaxProfilePoints bounds the size of a single profile response.
const MaxProfilePoints = 100000

// PointResult is the outcome of one sampled profile point: either the full
// atmosphere state or the reason the sample was rejected (for example
// because a temperature offset pushed its density altitude outside the
// modelled layers). Exactly one of Atmosphere/Err is set; Altitude is always
// the sampled geometric altitude.
type PointResult struct {
	Altitude   float64
	Atmosphere Atmosphere
	Err        error
}

// Profile evaluates the atmosphere at the evenly spaced altitudes
// start, start+step, ... not exceeding end. Points are generated with integer
// indexing to avoid floating-point accumulation drift. If end is not an exact
// multiple of step above start, the last returned point is the largest
// multiple below end (the endpoint is not stretched).
//
// Profile is strict: the first sample the model refuses aborts the whole
// profile. Use ProfilePoints when individual sample failures should be
// reported alongside the valid points instead.
func Profile(start, end, step, deltaT float64) ([]Atmosphere, error) {
	results, err := ProfilePoints(start, end, step, deltaT)
	if err != nil {
		return nil, err
	}
	points := make([]Atmosphere, 0, len(results))
	for _, r := range results {
		if r.Err != nil {
			return nil, r.Err
		}
		points = append(points, r.Atmosphere)
	}
	return points, nil
}

// ProfilePoints samples the atmosphere exactly like Profile but evaluates
// every point independently: a sample the model refuses is reported in that
// sample's Err (with its Altitude) while the remaining samples are still
// returned — one out-of-range point neither leaks a wild value into the
// results nor condemns the rest of the batch. Structural problems (invalid
// interval, step, offset, or too many points) still fail the whole call.
func ProfilePoints(start, end, step, deltaT float64) ([]PointResult, error) {
	if err := validateAltitude(start); err != nil {
		return nil, InvalidArgumentError("start: " + err.Error())
	}
	if err := validateAltitude(end); err != nil {
		return nil, InvalidArgumentError("end: " + err.Error())
	}
	switch {
	case math.IsNaN(step) || math.IsInf(step, 0) || step <= 0:
		return nil, InvalidArgumentError("step must be a finite positive number of metres")
	case end < start:
		return nil, InvalidArgumentError("end altitude must be greater than or equal to start altitude")
	case math.IsNaN(deltaT) || math.IsInf(deltaT, 0):
		return nil, InvalidArgumentError("temperature offset must be a finite number")
	}

	count := int((end-start)/step) + 1 // number of multiples of step in [start, end]
	if count > MaxProfilePoints {
		return nil, InvalidArgumentError("profile would contain more than 100000 points; widen the step or shrink the interval")
	}

	results := make([]PointResult, 0, count)
	for i := 0; i < count; i++ {
		h := start + float64(i)*step
		if h > end { // guard against rounding at the upper end
			break
		}
		a, err := Model(h, deltaT)
		if err != nil {
			results = append(results, PointResult{Altitude: h, Err: err})
			continue
		}
		results = append(results, PointResult{Altitude: h, Atmosphere: a})
	}
	return results, nil
}
