package kit

import "testing"

func TestCentsToDecimal(t *testing.T) {
	cases := map[int64]string{
		3499:  "34.99",
		100:   "1.00",
		5:     "0.05",
		0:     "0.00",
		-5:    "-0.05",
		-3499: "-34.99",
	}
	for cents, want := range cases {
		if got := centsToDecimal(cents); got != want {
			t.Errorf("centsToDecimal(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestUpper(t *testing.T) {
	cases := map[string]string{
		"usd": "USD",
		"PLN": "PLN",
		"uSd": "USD",
		"":    "",
	}
	for input, want := range cases {
		if got := upper(input); got != want {
			t.Errorf("upper(%q) = %q, want %q", input, got, want)
		}
	}
}
