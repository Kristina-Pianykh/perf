package gh

import (
	"fmt"
	"os"

	"github.com/google/go-github/v72/github"
)

func InitClient() (*github.Client, error) {
	token, ok := os.LookupEnv("GITHUB_API_TOKEN")
	if !ok {
		return nil, fmt.Errorf("missing GITHUB_API_TOKEN")
	}
	client := github.NewClient(nil).WithAuthToken(token)
	return client, nil
}
