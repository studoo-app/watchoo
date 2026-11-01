package store

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	paris := time.FixedZone("UTC+1", 3600)
	india := time.FixedZone("UTC+5:30", 5*3600+30*60)
	newYork := time.FixedZone("UTC-4", -4*3600)
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"UTC", time.Date(2026, 10, 26, 14, 3, 0, 0, time.UTC), "2026-10-26T14:03:00Z"},
		{"positive time zone, truncated fraction", time.Date(2026, 10, 26, 15, 3, 0, 900_000_000, paris), "2026-10-26T14:03:00Z"},
		{"half-hour offset", time.Date(2026, 10, 26, 19, 33, 59, 999_999_999, india), "2026-10-26T14:03:59Z"},
		{"negative time zone, day change", time.Date(2026, 12, 31, 22, 0, 1, 0, newYork), "2027-01-01T02:00:01Z"},
	}
	for _, tt := range tests {
		if got := FormatTime(tt.in); got != tt.want {
			t.Errorf("%s: FormatTime(%v) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestParseTime(t *testing.T) {
	got, err := ParseTime("2026-10-26T14:03:00Z")
	if err != nil {
		t.Fatalf("ParseTime: %v", err)
	}
	want := time.Date(2026, 10, 26, 14, 3, 0, 0, time.UTC)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("ParseTime = %v (%v), want %v in UTC", got, got.Location(), want)
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	zone := time.FixedZone("UTC+2", 2*3600)
	for _, in := range []time.Time{
		time.Date(2026, 10, 26, 16, 3, 7, 123_456_789, zone),
		time.Date(2024, 2, 29, 23, 59, 59, 0, time.UTC),
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Now(),
	} {
		s := FormatTime(in)
		got, err := ParseTime(s)
		if err != nil {
			t.Fatalf("ParseTime(%q): %v", s, err)
		}
		want := in.UTC().Truncate(time.Second)
		if !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("round trip of %v: %v (%v), want %v in UTC", in, got, got.Location(), want)
		}
		if FormatTime(got) != s {
			t.Errorf("round trip of %v: %q then %q", in, s, FormatTime(got))
		}
	}
}

func TestFormatTimeSortsAsText(t *testing.T) {
	earlier := FormatTime(time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC))
	later := FormatTime(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if !(earlier < later) {
		t.Errorf("%q should come before %q in text order", earlier, later)
	}
}

func TestParseTimeRejectsInvalid(t *testing.T) {
	for _, in := range []string{
		"",
		"2026-10-26T14:03:00",           // without Z
		"2026-10-26T14:03:00z",          // lowercase z
		"2026-10-26 14:03:00Z",          // space instead of T
		"2026-10-26T14:03:00.5Z",        // fraction of a second
		"2026-10-26T14:03:00.000000Z",   // zero fraction
		"2026-10-26T14:03:00+02:00",     // time offset
		"2026-10-26T14:03:00+00:00",     // zero offset
		"2026-10-26T4:03:00Z",           // one-digit hour
		"2026-02-30T00:00:00Z",          // nonexistent date
		"2026-10-26T24:00:00Z",          // hour out of range
		" 2026-10-26T14:03:00Z",         // leading space
		"2026-10-26T14:03:00Z ",         // trailing space
		"26/10/2026 14:03",              // other format
		"2026-10-26T14:03:00Z\x00extra", // extra characters
	} {
		got, err := ParseTime(in)
		if err == nil {
			t.Errorf("ParseTime(%q) = %v, error expected", in, got)
			continue
		}
		if !got.IsZero() {
			t.Errorf("ParseTime(%q): value %v returned with the error, want the zero value", in, got)
		}
		// The error quotes the received value (in quotes, special characters escaped).
		if in != "" && !strings.Contains(err.Error(), strconv.Quote(in)) {
			t.Errorf("ParseTime(%q): the error %q does not quote the received value", in, err)
		}
	}
}

func TestParseTimeErrorQuotesValue(t *testing.T) {
	_, err := ParseTime("2026-10-26T14:03:00+02:00")
	if err == nil {
		t.Fatal("error expected")
	}
	if !strings.Contains(err.Error(), `"2026-10-26T14:03:00+02:00"`) || !strings.Contains(err.Error(), TimeLayout) {
		t.Errorf("error %q: should quote the received value and the expected format", err)
	}
	_, err = ParseTime("")
	if err == nil || !strings.Contains(err.Error(), "empty date") {
		t.Errorf("ParseTime(\"\"): error %v, want an \"empty date\" message", err)
	}
}
