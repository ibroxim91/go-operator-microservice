package repository

import (
    "database/sql"
    "strings"
)

type Hotel struct {
    ID   int
    Name string
}

type HotelMapping struct {
    ID           int
    Operator     string
    OperatorHotelID int
    HotelID      int
}

type HotelPhoto struct {
    URL   string
    Note  string
    Count int // qo‘shimcha field
}


func GetHotelData(db *sql.DB, hotelID int, hotelName, operator string, countryID int, mealPlan string) (*Hotel, bool, error) {
    // Avval mappingdan qidiramiz
    hotel, err := GetHotelFromMapping(db, hotelID, operator)
    exists := true
    if err != nil || hotel == nil {
        exists = false
        // Agar mapping topilmasa → country_id bo‘yicha qidirish
        hotel, err = FindHotelByName(db, countryID, hotelName)
        if err != nil {
            return nil, exists, err
        }
        // Topilgan hotelni mapping jadvaliga saqlash
        _ = SaveHotelMapping(db, operator, hotelID, hotel.ID)
    }
    return hotel, exists, nil
}



func GetHotelPhoto(db *sql.DB, hotelID int) (*HotelPhoto, error) {
    // 1. Birinchi rasmni olish
    query := `SELECT url, note FROM api_hotelphoto WHERE hotel_id = $1 ORDER BY id ASC LIMIT 1`
    row := db.QueryRow(query, hotelID)

    var photo HotelPhoto
    err := row.Scan(&photo.URL, &photo.Note)
    if err != nil {
        return nil, err
    }

    // 2. Rasmlar sonini olish
    countQuery := `SELECT COUNT(*) FROM api_hotelphoto WHERE hotel_id = $1`
    err = db.QueryRow(countQuery, hotelID).Scan(&photo.Count)
    if err != nil {
        return nil, err
    }

    return &photo, nil
}

// get_hotel_from_mapping: avval mappingdan qidiradi
func GetHotelFromMapping(db *sql.DB, hotelID int, operator string) (*Hotel, error) {
    query := `SELECT h.id, h.name 
              FROM hotel_mapping m 
              JOIN api_hotel h ON m.hotel_id = h.id
              WHERE m.operator = $1 AND m.operator_hotel_id = $2 LIMIT 1`

    row := db.QueryRow(query, operator, hotelID)
    var hotel Hotel
    err := row.Scan(&hotel.ID, &hotel.Name)
    if err != nil {
        return nil, err
    }
    return &hotel, nil
}

func FindHotelByName(db *sql.DB, countryID int, hotelName string) (*Hotel, error) {
    query := `SELECT id, name FROM api_hotel WHERE country_id = $1`
    rows, err := db.Query(query, countryID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    hotelName = strings.ToLower(strings.TrimSpace(hotelName))
    var hotel Hotel
    for rows.Next() {
        var h Hotel
        if err := rows.Scan(&h.ID, &h.Name); err != nil {
            continue
        }
        hName := strings.ToLower(strings.TrimSpace(h.Name))
        if strings.Contains(hName, hotelName) || strings.Contains(hotelName, hName) {
            hotel = h
            break
        }
    }
    if hotel.ID == 0 {
        return nil, sql.ErrNoRows
    }
    return &hotel, nil
}


func SaveHotelMapping(db *sql.DB, operator string, operatorHotelID int, hotelID int) error {
    query := `INSERT INTO hotel_mapping (operator, operator_hotel_id, hotel_id) 
              VALUES ($1, $2, $3)`
    _, err := db.Exec(query, operator, operatorHotelID, hotelID)
    return err
}



