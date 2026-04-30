package utils

import (
	"encoding/json"
	"go-operator-service/models"
	"os"
)



func SavePricesToFile(prices []*models.Ticket, filename string) error {
    file, err := os.Create(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")
    encoder.SetEscapeHTML(false) // <-- bu joy muhim
    return encoder.Encode(prices)
}
