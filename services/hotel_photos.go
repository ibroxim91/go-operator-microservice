package services

import (
	"database/sql"
	"sync"
	"time"

	"go-operator-service/models"
	"go-operator-service/repository"
)

// Suratlar keshining umri.
//
// Avval kesh abadiy edi: mehmonxona bir marta suratsiz deb yozilsa,
// servis qayta ishga tushmaguncha shunday qolardi. Kontent importi
// surat qo'shganda ular saytda ko'rinmasdi va har safar qo'lda
// restart kerak bo'lardi.
//
// O'n besh daqiqa — ikki narsa o'rtasidagi muvozanat: baza har
// so'rovda bezovta qilinmaydi, yangi surat esa yarim soat ichida
// o'zi paydo bo'ladi.
const hotelPhotosTTL = 15 * time.Minute

type cachedPhotos struct {
	photos  []models.HotelPhotos
	savedAt time.Time
}

var (
	hotelPhotosCache   = map[int]cachedPhotos{}
	hotelPhotosCacheMu sync.RWMutex
)

func getCachedHotelPhotos(db *sql.DB, hotelID int) ([]models.HotelPhotos, error) {
	hotelPhotosCacheMu.RLock()
	entry, ok := hotelPhotosCache[hotelID]
	hotelPhotosCacheMu.RUnlock()
	if ok && time.Since(entry.savedAt) < hotelPhotosTTL {
		return entry.photos, nil
	}

	photos, err := repository.GetHotelPhotos(db, hotelID)
	if err != nil {
		// Baza javob bermasa eski nusxa yo'qligidan ko'ra yaxshiroq.
		if ok {
			return entry.photos, nil
		}
		return nil, err
	}

	hotelPhotosCacheMu.Lock()
	hotelPhotosCache[hotelID] = cachedPhotos{photos: photos, savedAt: time.Now()}
	hotelPhotosCacheMu.Unlock()

	return photos, nil
}
