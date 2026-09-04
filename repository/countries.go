package repository

import (
	"database/sql"
)

// GetCountryVisaRequiredMap returns country_id -> visa_required.
func GetCountryVisaRequiredMap(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query(`
		SELECT id, COALESCE(visa_required, false)
		FROM countries
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]bool)
	for rows.Next() {
		var id int
		var visaRequired bool
		if err := rows.Scan(&id, &visaRequired); err != nil {
			continue
		}
		result[id] = visaRequired
	}
	return result, rows.Err()
}
