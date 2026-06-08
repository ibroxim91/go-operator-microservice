package models
type BoolString bool

func (b *BoolString) UnmarshalJSON(data []byte) error {
    s := string(data)
    switch s {
    case `"1"`, `1`:
        *b = true
    case `"0"`, `0`:
        *b = false
    case `true`:
        *b = true
    case `false`:
        *b = false
    default:
        *b = false
    }
    return nil
}

type Ticket struct {
    TourOperatorID string         `json:"tour_operator_id"`
    ID             int            `json:"id"`
    Title          string         `json:"title"`
    Slug           string         `json:"slug"`
    Nights         int            `json:"nights"`
    Price          string         `json:"price"`
    PriceFull      int            `json:"price_full"`
    Operator       string         `json:"operator"`
    DepartureID    int            `json:"departure_id"`
    DestinationID  int            `json:"destination_id"`
    CountryID      int            `json:"country_id"`
    DepartureTime  string         `json:"departure_time"`
    HotelPhoto     string         `json:"hotel_photo"`
    Currency       string         `json:"currency"`
    HotelAvailability   string    `json:"hotelAvailability"`
    HotelPhotoCount     int        `json:"hotel_photo_count"`
    HotelPhotos     []HotelPhotos  `json:"hotel_photos"`
    Departure      DepartureInfo  `json:"departure"`
    PassengerCount int            `json:"passenger_count"`
    Rating         float64        `json:"rating"`
    DurationDays   int            `json:"duration_days"`
    Destination    DestinationInfo `json:"destination"`
    TicketImages   string         `json:"ticket_images"`
    TicketAmenities []string      `json:"ticket_amenities"`
    Badge          []string       `json:"badge"`
    VisaRequired   bool           `json:"visa_required"`
    FromCache      bool           `json:"from_cache"`
    IsLiked        bool           `json:"is_liked"`
    TicketHotel    []TicketHotel  `json:"ticket_hotel"`

    // Qo‘shimcha fieldlar
    DepartureDate          string            `json:"departure_date"`
    RoomType               string            `json:"room_type"`
    Place                  string            `json:"place"`
    FreightExternal     string               `json:"freight_external"`
    TravelTime             string            `json:"travel_time"`
    Languages              string            `json:"languages"`
    MinPerson              int               `json:"min_person"`
    MaxPerson              int               `json:"max_person"`
    ImageBanner            string            `json:"image_banner"`
    HotelInfo              string            `json:"hotel_info"`
    HotelMeals             string            `json:"hotel_meals"`
    AllowComment           bool              `json:"allow_comment"`
    Bron                   BoolString              `json:"bron"`
    TicketIncludedServices []IncludedService `json:"ticket_included_services"`
    TicketItinerary        []string          `json:"ticket_itinerary"`
    TicketHotelMeals       []HotelMeal       `json:"ticket_hotel_meals"`
    TravelAgencyID         string            `json:"travel_agency_id"`
    TicketComments         []string          `json:"ticket_comments"`
    Tariff                 []Tariff          `json:"tariff"`
    Transports             []Transport       `json:"transports"`
    ExtraService           []string          `json:"extra_service"`
    PaidExtraService       []string          `json:"paid_extra_service"`
}


type HotelPhotos struct {
    Image string `json:"image"`
}

type IncludedService struct {
    Image string `json:"image"`
    Title string `json:"title"`
    Desc  string `json:"desc"`
}

type HotelMeal struct {
    Image string `json:"image"`
    Name  string `json:"name"`
    Desc  string `json:"desc"`
}

type Tariff struct {
    Name string `json:"name"`
}

type Transport struct {
    ID  int `json:"id"`
    Name string `json:"name"`
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


type StreamCacheResult struct {
    Tickets []*Ticket       `json:"tickets"`
    Hotels  []HotelSummary  `json:"hotels"`
    Total   int             `json:"total"`
}