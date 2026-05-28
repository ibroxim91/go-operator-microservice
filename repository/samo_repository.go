package repository

import (
	"database/sql"
)

type CountryMapping struct {
	CountryID           int
	OperatorStateID     int
	CountryName         string
	DestinationImageURL string
}

type RegionMapping struct {
	RegionID       int
	OperatorTownID int
}

type TownMapping struct {
	TownID         int
	OperatorTownID int
}

type RegionInfo struct {
	ID            int
	CountryID     int
	CountryNameRu string
	NameRu        string
	NameUz        string
}

type HotelMealPlanMapping struct {
	MealPlanID int
	MealKey    string
}

type HotelRatingMapping struct {
	Rating    string
	RatingKey string
}

type QueryCache struct {
	QueryKey            string
	OperatorName        string
	URL                 string
	DestinationImageURL string
}

func GetCountryMapping(db *sql.DB, operator string, stateID int) (*CountryMapping, error) {
	query := `SELECT cm.country_id,
				cm.operator_state_id,
				c.name AS country_name,
				c.image AS destination_image_url
			FROM api_countrymapping cm
			JOIN countries c ON cm.country_id = c.id
			WHERE cm.operator = $1 AND cm.country_id = $2
			LIMIT 1;
`

	row := db.QueryRow(query, operator, stateID)
	mapping := CountryMapping{}
	if err := row.Scan(&mapping.CountryID, &mapping.OperatorStateID, &mapping.CountryName, &mapping.DestinationImageURL); err != nil {
		return nil, err
	}
	return &mapping, nil
}

func GetRegionMapping(db *sql.DB, operator string, regionID int) (*RegionMapping, error) {
	query := `SELECT region_id, operator_town_id
              FROM api_regionmapping
              WHERE operator = $1 AND region_id = $2
              LIMIT 1`

	row := db.QueryRow(query, operator, regionID)
	mapping := RegionMapping{}
	if err := row.Scan(&mapping.RegionID, &mapping.OperatorTownID); err != nil {
		return nil, err
	}
	return &mapping, nil
}

func GetRegionByID(db *sql.DB, regionID int) (*RegionInfo, error) {
	query := `SELECT r.id, r.country_id, r.name_ru, r.name_uz, c.name_ru
              FROM regions r
              LEFT JOIN countries c ON c.id = r.country_id
              WHERE r.id = $1
              LIMIT 1`

	row := db.QueryRow(query, regionID)
	region := RegionInfo{}
	if err := row.Scan(&region.ID, &region.CountryID, &region.NameRu, &region.NameUz, &region.CountryNameRu); err != nil {
		return nil, err
	}
	return &region, nil
}

func GetTownMapping(db *sql.DB, operator string, townID int) (*TownMapping, error) {
	query := `SELECT town_id, operator_town_id
              FROM api_townmapping
              WHERE operator = $1 AND town_id = $2
              LIMIT 1`

	row := db.QueryRow(query, operator, townID)
	mapping := TownMapping{}
	if err := row.Scan(&mapping.TownID, &mapping.OperatorTownID); err != nil {
		return nil, err
	}
	return &mapping, nil
}

func GetTownMappingsByRegion(db *sql.DB, operator string, regionID int) ([]TownMapping, error) {
	query := `SELECT town_id, operator_town_id
              FROM api_townmapping
              WHERE operator = $1 AND region_id = $2`

	rows, err := db.Query(query, operator, regionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []TownMapping
	for rows.Next() {
		var mapping TownMapping
		if err := rows.Scan(&mapping.TownID, &mapping.OperatorTownID); err != nil {
			continue
		}
		mappings = append(mappings, mapping)
	}
	return mappings, nil
}

func GetMealPlanMapping(db *sql.DB, operator string, mealID int) (*HotelMealPlanMapping, error) {
	query := `SELECT meal_plan_id, meal_key
              FROM meal_plan_mapping
              WHERE operator = $1 AND meal_plan_id = $2
              LIMIT 1`

	row := db.QueryRow(query, operator, mealID)
	mapping := HotelMealPlanMapping{}
	if err := row.Scan(&mapping.MealPlanID, &mapping.MealKey); err != nil {
		return nil, err
	}
	return &mapping, nil
}

func GetRatingMapping(db *sql.DB, operator string, rating string) (*HotelRatingMapping, error) {
	query := `SELECT rating, rating_key
              FROM rating_mapping
              WHERE operator = $1 AND rating = $2
              LIMIT 1`

	row := db.QueryRow(query, operator, rating)
	mapping := HotelRatingMapping{}
	if err := row.Scan(&mapping.Rating, &mapping.RatingKey); err != nil {
		return nil, err
	}
	return &mapping, nil
}

func GetQueryCache(db *sql.DB, queryKey, operator string) (*QueryCache, error) {
	query := `SELECT query_key, operator_name, url, destination_image_url
              FROM api_querycache
              WHERE query_key = $1 AND operator_name = $2
              LIMIT 1`

	row := db.QueryRow(query, queryKey, operator)
	cache := QueryCache{}
	if err := row.Scan(&cache.QueryKey, &cache.OperatorName, &cache.URL, &cache.DestinationImageURL); err != nil {
		return nil, err
	}
	return &cache, nil
}

func SaveQueryCache(db *sql.DB, queryKey, operator, url, destinationImageURL string) error {
	// Delete PricePage parametrini saqlashdan oldin, chunki u har doim o‘zgarib turadi va cache samaradorligini pasaytiradi

	query := `INSERT INTO api_querycache (query_key, operator_name, url, destination_image_url, created_at)
          VALUES ($1, $2, $3, $4, NOW())`
	_, err := db.Exec(query, queryKey, operator, url, destinationImageURL)

	return err
}
