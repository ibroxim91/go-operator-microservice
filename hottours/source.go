package hottours

import (
	"os"
	"strings"
)

const (
	// SourceSAMO serves cheapest SAMO home-offers as hot tours.
	SourceSAMO = "samo"
	// SourceTourvisor serves Tourvisor /tours/hots.
	SourceTourvisor = "tourvisor"
)

// Source is HOT_TOURS_SOURCE: samo (default) or tourvisor.
func Source() string {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("HOT_TOURS_SOURCE")))
	switch raw {
	case SourceTourvisor, "tv":
		return SourceTourvisor
	default:
		return SourceSAMO
	}
}

func IsTourvisor() bool {
	return Source() == SourceTourvisor
}
