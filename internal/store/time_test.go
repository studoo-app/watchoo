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
		{"fuseau positif, fraction tronquée", time.Date(2026, 10, 26, 15, 3, 0, 900_000_000, paris), "2026-10-26T14:03:00Z"},
		{"demi-heure de décalage", time.Date(2026, 10, 26, 19, 33, 59, 999_999_999, india), "2026-10-26T14:03:59Z"},
		{"fuseau négatif, changement de jour", time.Date(2026, 12, 31, 22, 0, 1, 0, newYork), "2027-01-01T02:00:01Z"},
	}
	for _, tt := range tests {
		if got := FormatTime(tt.in); got != tt.want {
			t.Errorf("%s : FormatTime(%v) = %q, attendu %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestParseTime(t *testing.T) {
	got, err := ParseTime("2026-10-26T14:03:00Z")
	if err != nil {
		t.Fatalf("ParseTime : %v", err)
	}
	want := time.Date(2026, 10, 26, 14, 3, 0, 0, time.UTC)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("ParseTime = %v (%v), attendu %v en UTC", got, got.Location(), want)
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
			t.Fatalf("ParseTime(%q) : %v", s, err)
		}
		want := in.UTC().Truncate(time.Second)
		if !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("aller-retour de %v : %v (%v), attendu %v en UTC", in, got, got.Location(), want)
		}
		if FormatTime(got) != s {
			t.Errorf("aller-retour de %v : %q puis %q", in, s, FormatTime(got))
		}
	}
}

func TestFormatTimeSortsAsText(t *testing.T) {
	earlier := FormatTime(time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC))
	later := FormatTime(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if !(earlier < later) {
		t.Errorf("%q devrait précéder %q dans l'ordre du texte", earlier, later)
	}
}

func TestParseTimeRejectsInvalid(t *testing.T) {
	for _, in := range []string{
		"",
		"2026-10-26T14:03:00",           // sans Z
		"2026-10-26T14:03:00z",          // z minuscule
		"2026-10-26 14:03:00Z",          // espace au lieu de T
		"2026-10-26T14:03:00.5Z",        // fraction de seconde
		"2026-10-26T14:03:00.000000Z",   // fraction nulle
		"2026-10-26T14:03:00+02:00",     // décalage horaire
		"2026-10-26T14:03:00+00:00",     // décalage nul
		"2026-10-26T4:03:00Z",           // heure sur un chiffre
		"2026-02-30T00:00:00Z",          // date inexistante
		"2026-10-26T24:00:00Z",          // heure hors limites
		" 2026-10-26T14:03:00Z",         // espace en tête
		"2026-10-26T14:03:00Z ",         // espace en fin
		"26/10/2026 14:03",              // autre format
		"2026-10-26T14:03:00Z\x00extra", // caractères en trop
	} {
		got, err := ParseTime(in)
		if err == nil {
			t.Errorf("ParseTime(%q) = %v, erreur attendue", in, got)
			continue
		}
		if !got.IsZero() {
			t.Errorf("ParseTime(%q) : valeur %v renvoyée avec l'erreur, attendu la valeur zéro", in, got)
		}
		// L'erreur cite la valeur reçue (entre guillemets, caractères spéciaux échappés).
		if in != "" && !strings.Contains(err.Error(), strconv.Quote(in)) {
			t.Errorf("ParseTime(%q) : l'erreur %q ne cite pas la valeur reçue", in, err)
		}
	}
}

func TestParseTimeErrorQuotesValue(t *testing.T) {
	_, err := ParseTime("2026-10-26T14:03:00+02:00")
	if err == nil {
		t.Fatal("erreur attendue")
	}
	if !strings.Contains(err.Error(), `"2026-10-26T14:03:00+02:00"`) || !strings.Contains(err.Error(), TimeLayout) {
		t.Errorf("erreur %q : devrait citer la valeur reçue et le format attendu", err)
	}
	_, err = ParseTime("")
	if err == nil || !strings.Contains(err.Error(), "vide") {
		t.Errorf("ParseTime(\"\") : erreur %v, attendu un message « date vide »", err)
	}
}
