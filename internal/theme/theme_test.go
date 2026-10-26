package theme

import (
	"strings"
	"testing"
)

func TestAllParses(t *testing.T) {
	all, errs := All()
	if len(all) <= 600 {
		t.Fatalf("%d schemes parsed, want > 600", len(all))
	}
	perSystem := map[string]int{}
	for _, s := range all {
		perSystem[s.System]++
	}
	t.Logf("parsed per system %v, %d errors", perSystem, len(errs))
	for _, e := range errs {
		t.Logf("error: %v", e)
	}
	if perSystem["tinted8"] != 11 {
		t.Fatalf("tinted8 parsed %d, want 11", perSystem["tinted8"])
	}
	if len(errs) != 0 {
		t.Fatalf("%d files failed to parse", len(errs))
	}
}

func TestByID(t *testing.T) {
	s, ok := ByID("base16/gruvbox-dark-hard")
	if !ok {
		t.Fatal("gruvbox-dark-hard not found")
	}
	if !strings.HasPrefix(s.Colors["base00"], "#") {
		t.Fatalf("base00 %q", s.Colors["base00"])
	}
}

func TestTinted8HasAllSlots(t *testing.T) {
	all, _ := All()
	for _, s := range all {
		if s.System != "tinted8" {
			continue
		}
		if len(s.Colors) == 16 {
			return
		}
		t.Fatalf("%s has %d slots", s.ID, len(s.Colors))
	}
	t.Fatal("no tinted8 scheme")
}
