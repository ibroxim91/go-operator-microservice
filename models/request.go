package models


type Request struct {
	Url string `json:"url"`
	Operator string `json:"operator"`
	Departure string `json:"departure"`
	DestinationID int `json:"destination_id"`
	DepartureID int `json:"departure_id"`
	CountryID int `json:"country_id"`
	DestCountryName string `json:"destination_country_name"`
	DestImageUrl string `json:"destination_image_url"`
	CurrentUsdCourse float64 `json:"current_usd_course"`
	Istest           bool `json:"test"`
	Page             int  `json:"page"`
	FirstPageOnly    bool `json:"first_page_only"`
	// MaxPages > 0 bo'lsa, shu sahifadan keyingisi olinmaydi (uzun sana
	// oralig'i bo'laklarga bo'linganda — services.SplitLongCheckinJobs).
	MaxPages int `json:"max_pages,omitempty"`
}