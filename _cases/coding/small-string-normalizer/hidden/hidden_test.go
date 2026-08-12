package normalize

import "testing"

func TestNormalizeKeyHidden(t *testing.T) {
	cases := map[string]string{
		"  Hello,   WORLD!! ": "hello-world",
		"---a__b///c---":      "a-b-c",
		"123 + 456":           "123-456",
		"!!!":                 "",
		"Crème brûlée":        "crème-brûlée",
		"日本 語":                "日本-語",
	}
	for in, want := range cases {
		if got := NormalizeKey(in); got != want {
			t.Errorf("NormalizeKey(%q)=%q want %q", in, got, want)
		}
	}
}
