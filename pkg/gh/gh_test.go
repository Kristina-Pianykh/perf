package gh

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTicket(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "[DX-57] feat: new feature", expected: "DX-57"},
		{input: "feat: new feature", expected: ""},
		{input: "feat: new feature [DX-57]", expected: "DX-57"},
		{input: "DX-57 feat: new feature", expected: "DX-57"},
		{input: "PF-5", expected: "PF-5"},
	}
	for _, tt := range tests {
		match := getTicket(tt.input)
		assert.Equal(t, tt.expected, match)
	}
}
