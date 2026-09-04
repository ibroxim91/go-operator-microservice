package services

import (
	"database/sql"
	"sync"

	"go-operator-service/logger"
	"go-operator-service/repository"
)

var (
	countryVisaMu   sync.RWMutex
	countryVisaMap  = map[int]bool{}
	countryVisaOnce sync.Once
)

// PreloadCountryVisaMap loads countries.visa_required into memory.
func PreloadCountryVisaMap(db *sql.DB) error {
	m, err := repository.GetCountryVisaRequiredMap(db)
	if err != nil {
		return err
	}
	countryVisaMu.Lock()
	countryVisaMap = m
	countryVisaMu.Unlock()
	logger.Log.Info().Int("countries", len(m)).Msg("country visa map preloaded")
	return nil
}

// RefreshCountryVisaMap reloads visa flags (used by home-offers warmup).
func RefreshCountryVisaMap(db *sql.DB) {
	if err := PreloadCountryVisaMap(db); err != nil {
		logger.Log.Warn().Err(err).Msg("failed to refresh country visa map")
	}
}

// IsCountryVisaRequired returns visa_required for internal country id (default false).
func IsCountryVisaRequired(countryID int) bool {
	if countryID <= 0 {
		return false
	}
	countryVisaMu.RLock()
	defer countryVisaMu.RUnlock()
	return countryVisaMap[countryID]
}

// EnsureCountryVisaMapLoaded loads once if empty.
func EnsureCountryVisaMapLoaded(db *sql.DB) {
	countryVisaOnce.Do(func() {
		if err := PreloadCountryVisaMap(db); err != nil {
			logger.Log.Warn().Err(err).Msg("failed to preload country visa map")
		}
	})
}
