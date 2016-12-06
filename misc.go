package main

// boolFloat returns float64 0 or 1 for passed bool
func boolFloat(inBool bool) float64 {
	if inBool {
		return 1
	}
	return 0
}
