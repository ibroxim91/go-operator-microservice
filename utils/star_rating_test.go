package utils

import "testing"

func TestStarRating(t *testing.T) {
	cases := map[string]float64{
		"5*":         5,
		"4*":         4,
		"3":          3,
		"5* DELUXE":  5,
		"3*+":        3,
		"3.5":        3.5,
		"4,5":        4.5,
		"****":       4,
		"HV-1":       1,
		"Boutique":   0,
		"VILLA":      0,
		"N/A":        0,
		"":           0,
		"0":          0,
		"Exc. Tour":  0,
		"Special Cl": 0,
	}
	for in, want := range cases {
		if got := StarRating(in); got != want {
			t.Errorf("StarRating(%q) = %v, want %v", in, got, want)
		}
	}
}
