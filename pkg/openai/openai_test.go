package openai

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenFile(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{
			path:    "./prompt",
			wantErr: false,
		},
		{
			path:    "./nonexistent",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		_, err := readFile(tt.path)
		assert.Equal(t, tt.wantErr, err != nil)
	}
}

func TestInit(t *testing.T) {
	_, err := InitClient()
	assert.NoError(t, err)
}

func TestChatCompletion(t *testing.T) {
	client, err := InitClient()
	assert.NoError(t, err)

	assert.NoError(t, err)
	input := "this is a test"
	output, err := Complete(client, context.Background(), &input)
	assert.NoError(t, err)
	assert.NotNil(t, output)
	fmt.Printf("output: %s\n", *output)
}
