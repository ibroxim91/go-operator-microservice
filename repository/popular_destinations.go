package repository

import (
	"database/sql"
)

type PopularDestination struct {
	ID              int
	RegionID        int
	ToRegionID      int
	CountryID       int
	DepartureName   string
	DestinationName string
	CountryName     string
	CountryImage    string
}

func GetPopularDestinations(db *sql.DB) ([]PopularDestination, error) {
	query := `
		SELECT
			pd.id,
			pd.region_id,
			pd.to_region_id,
			COALESCE(pd.country_id, dest.country_id) AS country_id,
			dep.name_ru,
			dest.name_ru,
			c.name_ru,
			COALESCE(c.image, '')
		FROM popular_destinations pd
		JOIN regions dep ON dep.id = pd.region_id
		LEFT JOIN regions dest ON dest.id = pd.to_region_id
		LEFT JOIN countries c ON c.id = COALESCE(pd.country_id, dest.country_id)
		WHERE pd.to_region_id IS NOT NULL
		ORDER BY pd.id
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	destinations := make([]PopularDestination, 0)
	for rows.Next() {
		var dest PopularDestination
		if err := rows.Scan(
			&dest.ID,
			&dest.RegionID,
			&dest.ToRegionID,
			&dest.CountryID,
			&dest.DepartureName,
			&dest.DestinationName,
			&dest.CountryName,
			&dest.CountryImage,
		); err != nil {
			continue
		}
		destinations = append(destinations, dest)
	}

	return destinations, rows.Err()
}
