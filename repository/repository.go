package repository

import (
	"context"
	"database/sql"
	"go-operator-service/models"

	"log"
	"sync"
)



type HotelMapping struct {
	Operator        string
	OperatorHotelID int
	HotelID         int
}

type HotelPhoto struct {
	URL   string
	Note  string
	Count int // qo‘shimcha field
}

var (
	cacheMu        sync.RWMutex


)


func SaveHotelMapping(
	ctx context.Context,
	db *sql.DB,
	operator string,
	operatorHotelID int,
	hotelID int,
) error {
	query := `
	INSERT INTO hotel_mapping(
		operator,
		operator_hotel_id,
		hotel_id
	)
	VALUES($1,$2,$3)
	ON CONFLICT (operator, operator_hotel_id) DO NOTHING
	`

	_, err := db.ExecContext(
		ctx,
		query,
		operator,
		operatorHotelID,
		hotelID,
	)
	return err
}

func GetHotelPhotos(db *sql.DB, hotelID int) ([]models.HotelPhotos, error) {

	query := `SELECT
			url
		FROM api_hotelphoto
		WHERE hotel_id = $1
		ORDER BY id
		`

	rows, err := db.Query(query, hotelID)
	if err != nil {
		log.Println("GetHotelPhotos error ", err)
		return nil, err
	}
	defer rows.Close()

	var photos []models.HotelPhotos

	for rows.Next() {

		var photo HotelPhoto

		err := rows.Scan(
			&photo.URL,
		)

		if err != nil {
			return nil, err
		}

		photos = append(photos, models.HotelPhotos{Image: photo.URL})
	}

	return photos, nil
}
