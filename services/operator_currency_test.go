package services

import "testing"

func TestUzsPerUsdReadsUsdBlock(t *testing.T) {
	body := []byte(`{
		"SearchTour_CURRENCIES": [
			{
				"currencyISO": "UZS",
				"rates": [{"value": "8.324E-5", "currencyISO": "USD"}]
			},
			{
				"currencyISO": "USD",
				"rates": [{"value": "12013", "currencyISO": "UZS"}]
			}
		]
	}`)
	rate, err := uzsPerUsd(body)
	if err != nil {
		t.Fatal(err)
	}
	if rate != 12013 {
		t.Fatalf("rate = %v", rate)
	}
}

func TestUzsPerUsdRejectsInverseOnly(t *testing.T) {
	body := []byte(`{"SearchTour_CURRENCIES":[{"currencyISO":"UZS","rates":[{"value":"8.324E-5","currencyISO":"USD"}]}]}`)
	if _, err := uzsPerUsd(body); err == nil {
		t.Fatal("inverse rate must not be used")
	}
}
