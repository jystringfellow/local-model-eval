package normalize

import "testing"

// TestNormalizeKeyHidden grades each input independently as a subtest so the
// harness can award per-artifact partial credit. Subtest names are stable ASCII
// labels (the inputs contain spaces, punctuation, and Unicode) so the verbose
// subtest output is readable in the diagnosis.
func TestNormalizeKeyHidden(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"whitespace-punct", "  Hello,   WORLD!! ", "hello-world"},
		{"run-collapsing", "---a__b///c---", "a-b-c"},
		{"alphanumeric", "123 + 456", "123-456"},
		{"no-alphanumerics", "!!!", ""},
		{"unicode-accents", "Crème brûlée", "crème-brûlée"},
		{"unicode-script", "日本 語", "日本-語"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeKey(c.in); got != c.want {
				t.Errorf("NormalizeKey(%q)=%q, want %q", c.in, got, c.want)
			}
		})
	}
}
