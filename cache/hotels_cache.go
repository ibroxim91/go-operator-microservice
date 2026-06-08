package cache

import "sync"

type Hotel struct {
	ID   int
	Name string
}

var HotelByIDCache = make(map[int]Hotel)
var HotelMappingCache = make(map[string]int)
var MappingCacheMu sync.RWMutex