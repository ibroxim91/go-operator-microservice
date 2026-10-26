package utils

import "testing"

func TestConvertOperatorPriceToUzsAppliesBonus(t *testing.T) {
	// Operator $1000 at 12013: 970 * 12013 = 11_652_610
	got := ConvertOperatorPriceToUzs(1000, 12013)
	want := 11_652_610
	if got != want {
		t.Fatalf("ConvertOperatorPriceToUzs(1000, 12013) = %d, want %d", got, want)
	}
}

func TestConvertOperatorPriceToUzsInvalidInputs(t *testing.T) {
	if ConvertOperatorPriceToUzs(0, 12013) != 0 {
		t.Fatal("zero operator price must return 0")
	}
	if ConvertOperatorPriceToUzs(1000, 0) != 0 {
		t.Fatal("zero course must return 0")
	}
}
