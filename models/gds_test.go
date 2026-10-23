package models

import "testing"

// Tour names taken from the operators' own SAMO catalogues (2026-10-15).
func TestIsGDSTour(t *testing.T) {
	cases := map[string]bool{
		"MY: Куала-Лумпур из Ташкента (GDS)":                            true,
		"ID: Бали из Ташкента через Куала-Лумпур (GDS: Batik Air)":      true,
		"Бали: из ТАШКЕНТ Динамик + ГДС (UZ)":                           true,
		"Лечебные туры (Centrum Air)GDS":                                true,
		"Куала Лумпур GDS Uzbekistan Airways":                           true,
		"MY: Куала-Лумпур из Ташкента (Soft Block: Batik Air)с багажом": false,
		"AE: Дубай из Ташкента (BLOCK: Flydubai)":                       false,
		"Отдых в ОАЭ (блок Fly Dubai)":                                  false,
		"НЯЧАНГ ИЗ ТАШКЕНТА ( QANOT SHARQ )":                            false,
		"": false,
	}
	for name, want := range cases {
		if got := IsGDSTour(name); got != want {
			t.Errorf("IsGDSTour(%q) = %v, want %v", name, got, want)
		}
	}
}
