package geo

import "testing"

func TestLookupColo(t *testing.T) {
	info, ok := LookupColo("sjc")
	if !ok {
		t.Fatalf("expected lookup success")
	}
	if info.City != "San Jose" {
		t.Fatalf("unexpected city %s", info.City)
	}
}
