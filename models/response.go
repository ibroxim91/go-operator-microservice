package models


type ResultResponse struct {
    Prices       []*Ticket     `json:"prices"`
	Total   int `json:"total"`
}

type Result struct {
    Prices       []*Ticket     `json:"prices"`
    Pager        Pager       `json:"pager"`
    Operator     string      `json:"operator"`
    Departure    string      `json:"departure"`
    DestinationID int        `json:"destination_id"`
    DepartureID   int        `json:"departure_id"`
    DestCountry   string        `json:"country"`
    Error        string      `json:"error,omitempty"`
}

type Pager struct {
    Current int `json:"current"`
    Total   int `json:"total"`
}
