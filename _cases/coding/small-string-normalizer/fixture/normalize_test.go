package normalize

import "testing"

func TestNormalizeKeyBasic(t *testing.T) {
	if got := NormalizeKey(" Project Atlas "); got != "project-atlas" {
		t.Fatalf("got %q", got)
	}
}
