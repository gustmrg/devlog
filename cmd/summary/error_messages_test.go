package summary

import (
	"strings"
	"testing"
)

func TestInvalidDateMessageIncludesFlagAndValue(t *testing.T) {
	_, err := getParsedDate("tomorrow")
	if err == nil {
		t.Fatal("invalid date did not return an error")
	}
	if got, want := err.Error(), `invalid --date value "tomorrow": expected YYYY-MM-DD`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Fatalf("error contains an unexpected newline: %q", err)
	}
}

func TestInvalidRangeDateIdentifiesFromFlag(t *testing.T) {
	_, _, _, err := resolveRange(false, false, "last-week", "")
	if err == nil {
		t.Fatal("invalid range did not return an error")
	}
	if got, want := err.Error(), `invalid --from value "last-week": expected YYYY-MM-DD`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}
