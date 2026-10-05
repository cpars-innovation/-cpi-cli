package ops

import "strings"

// CompareVersions compares two artifact versions segment by segment
// ("1.0.10" > "1.0.9"). Numeric segments compare as numbers; a non-numeric
// segment sorts after any number and non-numeric segments compare as strings;
// missing segments count as 0 ("1.0" == "1.0.0"). It returns -1, 0 or 1.
func CompareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		x, y := "0", "0"
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if c := compareSegment(x, y); c != 0 {
			return c
		}
	}
	return 0
}

func compareSegment(x, y string) int {
	xn, yn := isDigits(x), isDigits(y)
	switch {
	case xn && yn:
		// compare as numbers without overflow: strip leading zeros, then length
		x, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
		if len(x) != len(y) {
			return sign(len(x) - len(y))
		}
		return strings.Compare(x, y)
	case xn:
		return -1
	case yn:
		return 1
	}
	return strings.Compare(x, y)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
