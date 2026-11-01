package palette

import "testing"

func TestPalette(t *testing.T) {
	if !Valid("teal") || Valid("#ff0000") || Valid("") || Valid("accent") {
		t.Fatal("Valid")
	}
	if Default(0) != "blue" || Default(10) != "blue" || Default(3) != "violet" || Default(-1) != "blue" {
		t.Fatal("Default")
	}
}
