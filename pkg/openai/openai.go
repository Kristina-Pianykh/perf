package openai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func InitClient() (*openai.Client, error) {
	apiKey, ok := os.LookupEnv("OPENAI_API_KEY")
	if !ok {
		return nil, fmt.Errorf("missing OPENAI_API_KEY")
	}
	client := openai.NewClient(
		option.WithAPIKey(apiKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)
	return &client, nil

}

func readFile(path string) (*string, error) {
	clean := filepath.Clean(path)
	// fmt.Printf("clean path: %s\n", clean)
	var absPath string

	if !filepath.IsAbs(clean) {
		pwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		absPath = filepath.Join(pwd, clean)
	} else {
		absPath = clean
	}

	// fmt.Printf("abs path: %s\n", absPath)
	file, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	contents := string(file)
	return &contents, nil
}

func Complete(client *openai.Client, ctx context.Context, input *string) (*string, error) {
	prompt, err := readFile("./prompt")
	if err != nil {
		return nil, err
	}

	chatCompletion, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(*input),
			openai.SystemMessage(*prompt),
		},
		Model: openai.ChatModelGPT4_1Mini, // switch to nano if possible
	})
	if err != nil {
		panic(err.Error())
	}

	if len(chatCompletion.Choices) == 0 {
		return nil, fmt.Errorf("failed to complete chat with %w", err)
	}

	output := chatCompletion.Choices[0].Message.Content
	return &output, nil
}
