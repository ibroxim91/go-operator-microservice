package models

type Ticket struct {
    TourOperatorID string `json:"tour_operator_id"`
    ID             int    `json:"id"`
    Title          string `json:"title"`
    Slug           string `json:"slug"`
    Nights         int    `json:"nights"`
    Price          string `json:"price"`
    PriceFull      int    `json:"price_full"`
    Operator       string `json:"operator"`
    DepartureID    int    `json:"departure_id"`
    DestinationID  int    `json:"destination_id"`
    DepartureTime  string `json:"departure_time"`
    Departure      DepartureInfo `json:"departure"`
    PassengerCount int    `json:"passenger_count"`
    Rating         float64 `json:"rating"`
    DurationDays   int    `json:"duration_days"`
    Destination    DestinationInfo `json:"destination"`
    TicketImages   string `json:"ticket_images"`
    TicketAmenities []string `json:"ticket_amenities"`
    Badge          []string `json:"badge"`
    VisaRequired   bool   `json:"visa_required"`
    FromCache      bool   `json:"from_cache"`
    IsLiked        bool   `json:"is_liked"`
    TicketHotel    []TicketHotel `json:"ticket_hotel"`
}

type DepartureInfo struct {
    ID      int    `json:"id"`
    Name    string `json:"name"`
    Country string `json:"country"`
}

type DestinationInfo struct {
    ID      int    `json:"id"`
    Name    string `json:"name"`
    Country CountryInfo `json:"country"`
}

type CountryInfo struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

type TicketHotel struct {
    ID       int    `json:"id"`
    Name     string `json:"name"`
    MealPlan string `json:"meal_plan"`
    Rating   interface{} `json:"rating"` // int yoki string bo‘lishi mumkin
}
