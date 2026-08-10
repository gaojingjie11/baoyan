package main

import (
	"strings"
	"testing"
)

func TestDecodeSchoolsRejectsDuplicateID(t *testing.T) {
	_, err := decodeSchools(strings.NewReader(`{"schools":[{"id":1,"school":"A"},{"id":1,"school":"B"}]}`))
	if err == nil {
		t.Fatal("duplicate ID accepted")
	}
}

func TestDecodeSchoolsAcceptsStringAndNumericID(t *testing.T) {
	catalog, err := decodeSchools(strings.NewReader(`{"updated_at":"2026-08-10","schools":[{"id":1,"school":"A"},{"id":"b","school":"B"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(catalog.Schools[0].ID), "1"; got != want {
		t.Fatalf("first ID = %q, want %q", got, want)
	}
	if got, want := string(catalog.Schools[1].ID), "b"; got != want {
		t.Fatalf("second ID = %q, want %q", got, want)
	}
}
