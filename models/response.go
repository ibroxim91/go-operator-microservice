package models

type ResultResponse struct {
	Prices []*Ticket `json:"prices"`
	Total  int       `json:"total"`
	Page   int       `json:"page"`
}

type AsyncSamoResult struct {
	Status bool          `json:"status"`
	Data   AsyncSamoData `json:"data"`
}

type AsyncSamoData struct {
	Links       Links                  `json:"links"`
	TotalItems  int                    `json:"total_items"`
	TotalPages  int                    `json:"total_pages"`
	PageSize    int                    `json:"page_size"`
	Total       int                    `json:"total"`
	CurrentPage int                    `json:"current_page"`
	Results     AsyncSamoResultPayload `json:"results"`
}

type Links struct {
	Previous *string `json:"previous"`
	Next     *string `json:"next"`
}

type AsyncSamoResultPayload struct {
	Tickets             []*Ticket      `json:"tickets"`
	MinPrice            int            `json:"min_price"`
	MaxPrice            int            `json:"max_price"`
	Hotels              []HotelSummary `json:"hotels"`
	HotelAmenities      []string       `json:"hotel_amenities"`
	HotelFeaturesByType []string       `json:"hotel_features_by_type"`
	HotelTypes          []string       `json:"hotel_types"`
	TopDestinations     []string       `json:"top_destinations"`
	TopDuration         []string       `json:"top_duration"`
}

type HotelSummary struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	MealPlan    string      `json:"meal_plan"`
	Rating      interface{} `json:"rating"`
	Operator    string      `json:"operator"`
	Destination string      `json:"destination"`
}

type Result struct {
	Prices        []*Ticket `json:"prices"`
	Pager         Pager     `json:"pager"`
	Operator      string    `json:"operator"`
	Departure     string    `json:"departure"`
	DestinationID int       `json:"destination_id"`
	DepartureID   int       `json:"departure_id"`
	DestCountry   string    `json:"country"`
	Page          int       `json:"page"`
	Error         string    `json:"error,omitempty"`
}

type Pager struct {
	Current int `json:"current"`
	Total   int `json:"total"`
}
