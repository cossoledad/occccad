package api

import "testing"

func TestExchangeFormatsAndUploadLimit(t *testing.T) {
	if maxExchangeUploadBytes < 100*1024*1024 {
		t.Fatalf("exchange upload limit %d does not satisfy the 100 MiB product requirement", maxExchangeUploadBytes)
	}
	tests := []struct {
		input, format, extension string
		valid                    bool
	}{
		{"step", "STEP", ".step", true},
		{"STP", "STEP", ".step", true},
		{"brep", "BREP", ".brep", true},
		{"BRP", "BREP", ".brep", true},
		{"iges", "", "", false},
	}
	for _, test := range tests {
		format, extension, _, valid := exchangeFormat(test.input)
		if format != test.format || extension != test.extension || valid != test.valid {
			t.Fatalf("exchangeFormat(%q) = %q, %q, %v", test.input, format, extension, valid)
		}
	}
}
