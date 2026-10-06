package money

import "testing"

func TestFormat(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1.000", 1500000: "1.500.000", -25000: "-25.000"}
	for in, want := range cases {
		if got := Format(in); got != want {
			t.Errorf("Format(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	cases := map[string]int64{"1.500.000": 1500000, "Rp 75.000": 75000, "36000": 36000, "": 0, "10.000,00": 10000}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := Parse("abc"); err == nil {
		t.Error("Parse(abc) seharusnya error")
	}
}

func TestParseQty(t *testing.T) {
	if q, _ := ParseQty("1,5"); q != 1.5 {
		t.Errorf("ParseQty(1,5) = %v", q)
	}
	if _, err := ParseQty("0"); err == nil {
		t.Error("qty 0 seharusnya error")
	}
}
