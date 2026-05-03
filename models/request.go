package models


type Request struct {
	Url string `json:"url"`
	Operator string `json:"operator"`
	Departure string `json:"departure"`
	DestinationID int `json:"destination_id"`
	DepartureID int `json:"departure_id"`
	DestCountryName string `json:"destination_country_name"`
	DestImageUrl string `json:"destination_image_url"`
	CurrentUsdCourse float64 `json:"current_usd_course"`
	Istest bool `json:"test"`
}