package jirautils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseJiraTime(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{
			input:   "2025-05-12T15:21:38.463+0200",
			wantErr: false,
		},
		{
			input:   "2025-05-12",
			wantErr: true,
		},
		{
			input:   "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		parsed, err := parseJiraTime(tt.input)
		fmt.Printf("%v\n", parsed)
		assert.Equal(t, tt.wantErr, err != nil)
	}
}

func TestGetIssue(t *testing.T) {
	key := "DX-75"
	client, err := InitJiraClient()
	assert.NoError(t, err)

	jTicket, err := GetIssue(client, key)
	assert.NoError(t, err)
	assert.NotNil(t, jTicket)
}
