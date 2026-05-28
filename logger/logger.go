package logger


import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

func Init() {
	Log = zerolog.New(os.Stdout).
		With().
		Timestamp().
		Str("service", "echo-service").
		Logger()

	zerolog.TimeFieldFormat = time.RFC3339
}