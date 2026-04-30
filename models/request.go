package models


type Request struct {
	Url string `json:"url"`
	Operator string `json:"operator"`
	Departure string `json:"departure"`
	DestinationID int `json:"destination_id"`
	DepartureID int `json:"departure_id"`
	DestCountryName string `json:"destination_country_name"`
	DestImageUrl string `json:"destination_image_url"`
	Istest bool `json:"test"`
}