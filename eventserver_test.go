package main

import (
	"testing"
)

func TestParseEvent(t *testing.T) {
	want := map[string]bool{
		// Random bad message cases
		"":              false,
		"123224":        false,
		"abcdkjdhfjsdf": false,
		"|":             false,
		"12|||":         false,
		"|||":           false,

		// Follow message cases
		"12|F|21212|31212":                   true,
		"1234567890|F|1234567890|1234567891": true,
		"12|F":            false,
		"12|F|12":         false,
		"|F|12|13":        false,
		"1|F|12|12|12|12": false,
		"1|F||2":          false,
		"1|F|2|":          false,

		// Unfollow message cases
		"2|U|4|5":   true,
		"2|U|4":     false,
		"2|U|4|":    false,
		"2|U||4":    false,
		"|U|4|5":    false,
		"|U||":      false,
		"1|U|2|3|4": false,

		// Broadcast message cases
		"1|B":          true,
		"1234567890|B": true,
		"|B":           false,
		"1|B|2":        false,
		"1|B|2|3":      false,

		// Private message cases
		"1|P|12|11": true,
		"1|P||":     false,
		"|P||":      false,
		"1|P|2|3|":  false,
		"|P":        false,
		"1|P":       false,
		"1|P|2|":    false,

		// Status message cases
		"12|S|23":    true,
		"12|S|23|23": false,
		"12|S|":      false,
	}

	for msg := range want {
		_, err := parseEvent(msg)
		if err != nil && want[msg] {
			t.Errorf("parseEvent with event - %v: want ok, got %+v", msg, err)
		}
		if err == nil && !want[msg] {
			t.Errorf("parseEvent with event - %v: want not ok, got ok", msg)
		}
	}
}
