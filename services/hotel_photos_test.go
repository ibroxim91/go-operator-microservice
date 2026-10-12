package services

import (
	"testing"
	"time"

	"go-operator-service/models"
)

// Kesh eskirmasa, kontent importi qo'shgan suratlar saytda
// ko'rinmaydi va har safar qo'lda restart kerak bo'ladi.
func TestHotelPhotosCacheExpires(t *testing.T) {
	hotelPhotosCacheMu.Lock()
	hotelPhotosCache = map[int]cachedPhotos{}
	hotelPhotosCacheMu.Unlock()

	fresh := cachedPhotos{
		photos:  []models.HotelPhotos{{Image: "a.jpg"}},
		savedAt: time.Now(),
	}
	stale := cachedPhotos{
		photos:  []models.HotelPhotos{{Image: "b.jpg"}},
		savedAt: time.Now().Add(-hotelPhotosTTL - time.Minute),
	}

	hotelPhotosCacheMu.Lock()
	hotelPhotosCache[1] = fresh
	hotelPhotosCache[2] = stale
	hotelPhotosCacheMu.Unlock()

	if entry := hotelPhotosCache[1]; time.Since(entry.savedAt) >= hotelPhotosTTL {
		t.Fatal("yangi yozuv eskirgan deb hisoblandi")
	}
	if entry := hotelPhotosCache[2]; time.Since(entry.savedAt) < hotelPhotosTTL {
		t.Fatal("eski yozuv hali yaroqli deb hisoblandi")
	}
}

// Suratsiz mehmonxona ham keshlanadi: aks holda har so'rovda bazaga
// boriladi. Lekin u ham eskirishi kerak — import surat qo'shishi mumkin.
func TestEmptyResultIsCachedAndExpires(t *testing.T) {
	hotelPhotosCacheMu.Lock()
	hotelPhotosCache = map[int]cachedPhotos{
		3: {photos: nil, savedAt: time.Now().Add(-hotelPhotosTTL - time.Second)},
	}
	hotelPhotosCacheMu.Unlock()

	entry, ok := hotelPhotosCache[3]
	if !ok {
		t.Fatal("bo'sh natija keshlanmadi")
	}
	if time.Since(entry.savedAt) < hotelPhotosTTL {
		t.Fatal("bo'sh natija eskirmadi")
	}
}
