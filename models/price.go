package models

type Price struct {
    ID       string `json:"id"`
    CheckIn  string `json:"checkIn"`
    CheckOut string `json:"checkOut"`
    Nights   int    `json:"nights"`
    Adult    int    `json:"adult"`
    Child    int    `json:"child"`
    Tour     string `json:"tour"`
    Town     string `json:"town"`
    Hotel    string `json:"hotel"`
    Star     string `json:"star"`
    Meal     string `json:"meal"`
    Currency string `json:"currency"`
    Price    string `json:"price"`
    StateKey int    `json:"stateKey"`
    TourKey int    `json:"tourKey"`
    TownFromKey int  `json:"townFromKey"`
    TownKey int    `json:"townKey"`
    HotelKey int    `json:"hotelKey"`
    Note     string `json:"note"`
    Bron     bool `  json:"bron"`
}

type Page struct {
    Current int `json:"current"`
    Total int `json:"total"`

}

type SearchTourResponse struct {
    SearchTour_PRICES struct {
        Prices []Price `json:"prices"`
        Pager Pager `json:"pager"`
    } `json:"SearchTour_PRICES"`

}


