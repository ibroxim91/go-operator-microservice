package hottours

import "testing"

func TestSourceDefaultsToSAMO(t *testing.T) {
	t.Setenv("HOT_TOURS_SOURCE", "")
	if got := Source(); got != SourceSAMO {
		t.Fatalf("Source() = %q, want %q", got, SourceSAMO)
	}
}

func TestSourceTourvisor(t *testing.T) {
	t.Setenv("HOT_TOURS_SOURCE", "tourvisor")
	if got := Source(); got != SourceTourvisor {
		t.Fatalf("Source() = %q, want %q", got, SourceTourvisor)
	}
	t.Setenv("HOT_TOURS_SOURCE", "TV")
	if got := Source(); got != SourceTourvisor {
		t.Fatalf("Source() = %q, want %q", got, SourceTourvisor)
	}
}
