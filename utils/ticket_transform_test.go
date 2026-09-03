package utils

import "testing"

func TestConvertOperatorPriceToUzsAppliesBonusAndStreetRate(t *testing.T) {
	// Operator $1000, CB 11813.09 → street 12013.09
	// After 3% bonus: 970 * 12013.09 = 11_652_697.3 → 11652697
	got := ConvertOperatorPriceToUzs(1000, 11813.09)
	want := 11_652_697
	if got != want {
		t.Fatalf("ConvertOperatorPriceToUzs(1000, 11813.09) = %d, want %d", got, want)
	}
}

func TestConvertOperatorPriceToUzsInvalidInputs(t *testing.T) {
	if ConvertOperatorPriceToUzs(0, 11813.09) != 0 {
		t.Fatal("zero operator price must return 0")
	}
	if ConvertOperatorPriceToUzs(1000, 0) != 0 {
		t.Fatal("zero course must return 0")
	}
}
