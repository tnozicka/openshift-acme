package util

import (
	"testing"
)

func TestFirstNLines(t *testing.T) {
	tt := []struct {
		s        string
		n        int
		expected string
	}{
		{"", -1, ""},
		{"", 0, ""},
		{"", 1, ""},
		{"", 2, ""},
		{"", 3, ""},
		{"", 10, ""},
		{"alfa beta", -5, ""},
		{"alfa beta", -1, ""},
		{"alfa beta", 0, ""},
		{"alfa beta", 1, "alfa beta"},
		{"alfa beta", 2, "alfa beta"},
		{"alfa beta", 3, "alfa beta"},
		{"alfa beta", 10, "alfa beta"},
		{"a\nb", 0, ""},
		{"a\nb", 1, "a"},
		{"a\nb", 2, "a\nb"},
		{"a\nb", 3, "a\nb"},
		{"a\nb", 10, "a\nb"},
		{"a\nb\n", 0, ""},
		{"a\nb\n", 1, "a"},
		{"a\nb\n", 2, "a\nb"},
		{"a\nb\n", 3, "a\nb\n"},
		{"a\nb\n", 4, "a\nb\n"},
	}
	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			result := FirstNLines(tc.s, tc.n)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestMaxNCharacters(t *testing.T) {
	tt := []struct {
		s        string
		n        int
		expected string
	}{
		{"", -10, ""},
		{"", -2, ""},
		{"", -1, ""},
		{"", 0, ""},
		{"", 1, ""},
		{"", 2, ""},
		{"", 3, ""},
		{"", 10, ""},
		{"a b c\nd\n", -1, ""},
		{"a b c\nd\n", 0, ""},
		{"a b c\nd\n", 1, "a"},
		{"a b c\nd\n", 2, "a "},
		{"a b c\nd\n", 3, "a b"},
		{"a b c\nd\n", 4, "a b "},
		{"a b c\nd\n", 5, "a b c"},
		{"a b c\nd\n", 6, "a b c\n"},
		{"a b c\nd\n", 7, "a b c\nd"},
		{"a b c\nd\n", 8, "a b c\nd\n"},
		{"a b c\nd\n", 9, "a b c\nd\n"},
		{"a b c\nd\n", 10, "a b c\nd\n"},
		{"a b c\nd\n", 20, "a b c\nd\n"},
	}
	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			result := MaxNCharacters(tc.s, tc.n)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
