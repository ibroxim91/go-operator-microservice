package hottours

import "testing"

func TestSourceDefaultsToTourvisor(t *testing.T) {
	t.Setenv("HOT_TOURS_SOURCE", "")
	if got := Source(); got != SourceTourvisor {
		t.Fatalf("Source() = %q, want %q", got, SourceTourvisor)
	}
}

func TestSourceSAMO(t *testing.T) {
	t.Setenv("HOT_TOURS_SOURCE", "samo")
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
