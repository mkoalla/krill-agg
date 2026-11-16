package daily

import "testing"

func TestCommas(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 3245071: "3,245,071", -12345: "-12,345"} {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPercentileLine(t *testing.T) {
	for p, want := range map[float64]string{5.9: "`~05%` - <@1>", 53.69: "`~53%` - <@1>", 99.99: "`~99%` - <@1>", 100: "`~100%` - <@1>"} {
		if got := percentileLine("1", p); got != want {
			t.Errorf("percentileLine(%v) = %q, want %q", p, got, want)
		}
	}
}
