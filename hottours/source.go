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

// Source is HOT_TOURS_SOURCE: tourvisor (default) or samo.
func Source() string {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("HOT_TOURS_SOURCE")))
	switch raw {
	case SourceSAMO:
		return SourceSAMO
	default:
		return SourceTourvisor
	}
}

func IsTourvisor() bool {
	return Source() == SourceTourvisor
}
