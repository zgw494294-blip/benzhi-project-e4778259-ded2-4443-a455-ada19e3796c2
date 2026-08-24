package domain

import "math"

// ValidMeasurement is shared by import and domain validation code that handles numeric readings.
func ValidMeasurement(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
