package cache

import (
	"fmt"
	"sync"
)

type Hotel struct {
	ID   int
	Name string
}

var HotelByIDCache = make(map[int]Hotel)
var HotelMappingCache = make(map[string]int)
var MappingCacheMu sync.RWMutex

func HotelMappingKey(operator string, operatorHotelID int) string {
	return fmt.Sprintf("%s:%d", operator, operatorHotelID)
}

func GetMappedHotelID(operator string, operatorHotelID int) (int, bool) {
	key := HotelMappingKey(operator, operatorHotelID)
	MappingCacheMu.RLock()
	defer MappingCacheMu.RUnlock()
	id, ok := HotelMappingCache[key]
	return id, ok
}

func SetHotelMapping(operator string, operatorHotelID, hotelID int) {
	key := HotelMappingKey(operator, operatorHotelID)
	MappingCacheMu.Lock()
	HotelMappingCache[key] = hotelID
	MappingCacheMu.Unlock()
}
