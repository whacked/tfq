package extract

import "testing"

func TestLineCol(t *testing.T) {
	c := "ab\ncde\nf"
	cases := []struct {
		off, line, col int
	}{
		{0, 1, 1}, // 'a'
		{1, 1, 2}, // 'b'
		{3, 2, 1}, // 'c' (after first \n)
		{5, 2, 3}, // 'e'
		{7, 3, 1}, // 'f'
	}
	for _, tc := range cases {
		l, col := lineCol(c, tc.off)
		if l != tc.line || col != tc.col {
			t.Errorf("off=%d got (%d,%d) want (%d,%d)", tc.off, l, col, tc.line, tc.col)
		}
	}
}
