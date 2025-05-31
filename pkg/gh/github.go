package gh

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/v72/github"
)

type Commit struct {
	SHA       string
	Author    string
	Timestamp github.Timestamp
	Files     []*github.CommitFile
	Message   string
}

func (c *Commit) String(withChanges bool) string {
	if withChanges {
		return fmt.Sprintf("&Commit{SHA: %s, Author: %s, Timestamp: %s, Files: %v, Message: %s}", c.SHA, c.Author, c.Timestamp.String(), c.Files, c.Message)
	}
	return fmt.Sprintf("&Commit{SHA: %s, Author: %s, Timestamp: %s, Message: %s}", c.SHA, c.Author, c.Timestamp.String(), c.Message)
}

type PullRequest struct {
	Owner       string
	Repo        string
	Author      string
	Created     github.Timestamp
	Description string
	Title       string
	URL         string
	Commits     []*Commit
}

func (pr *PullRequest) String(verbose bool) string {
	if !verbose {
		return fmt.Sprintf("&PullRequest{Owner: %s, Repo: %s, Author: %s, Created: %s, Title: %s, URL: %s",
			pr.Owner, pr.Repo, pr.Author, pr.Created.String(), pr.Title, pr.URL)
	}

	var builder strings.Builder
	str := fmt.Sprintf("&PullRequest{Owner: %s, Repo: %s, Author: %s, Created: %s, Description: %s, Title: %s, URL: %s, Commits: ",
		pr.Owner, pr.Repo, pr.Author, pr.Created.String(), pr.Description, pr.Title, pr.URL)
	builder.WriteString(str)

	for _, c := range pr.Commits {
		builder.WriteString(fmt.Sprintf(" %s", c.String(verbose)))
	}

	return builder.String()
}

func (pr *PullRequest) GetPullRequestNumber() (int, error) {
	parts := strings.Split(pr.URL, "/")
	prNumStr := parts[len(parts)-1]
	prNum, err := strconv.Atoi(prNumStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s to int with: %w", prNumStr, err)
	}
	return prNum, nil

}

func InitClient() (*github.Client, error) {
	token, ok := os.LookupEnv("GITHUB_API_TOKEN")
	if !ok {
		return nil, fmt.Errorf("missing GITHUB_API_TOKEN")
	}
	client := github.NewClient(nil).WithAuthToken(token)
	return client, nil
}

func GetPullRequestsByTicket(client *github.Client, ctx context.Context, org, ticket, user string) ([]*PullRequest, error) {
	prs, err := GetOrgPullRequests(client, ctx, org, ticket, user)
	if err != nil {
		return nil, err
	}

	pullRequests := []*PullRequest{}
	for _, pr := range prs {
		repo := getRepoName(pr.GetRepositoryURL())
		owner := getOwner(pr.GetRepositoryURL())
		pullRequest := &PullRequest{
			Owner:       owner,
			Repo:        repo,
			Author:      pr.GetUser().GetLogin(),
			Created:     pr.GetCreatedAt(),
			Description: pr.GetBody(),
			Title:       pr.GetTitle(),
			URL:         pr.GetURL(),
		}

		commits, err := GetCommitsByPullRequest(client, ctx, pullRequest)
		if err != nil {
			return nil, err
		}
		pullRequest.Commits = commits
		pullRequests = append(pullRequests, pullRequest)
	}

	return pullRequests, nil
}

func GetOrgPullRequests(client *github.Client, ctx context.Context, org, ticket, user string) ([]*github.Issue, error) {
	opts := &github.SearchOptions{Sort: "created", Order: "asc"}
	query := fmt.Sprintf("org:%s %s type:pr author:%s", org, ticket, user)
	fmt.Printf("query: %q\n", query)
	result, _, err := client.Search.Issues(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to search for PRs with query %s: %w", query, err)
	}

	fmt.Printf("total PR with ticket %s: %d\n", ticket, result.GetTotal())

	for _, res := range result.Issues {
		fmt.Printf("found PR: %s\n", res.String())
	}
	return result.Issues, nil
}

func GetCommitsByPullRequest(client *github.Client, ctx context.Context, pr *PullRequest) ([]*Commit, error) {
	prNum, err := pr.GetPullRequestNumber()
	if err != nil {
		return nil, err
	}

	repoCommits, _, err := client.PullRequests.ListCommits(ctx, pr.Owner, pr.Repo, prNum, nil)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		return nil, fmt.Errorf("failed to fetch commits for Pull Request %s: %w", pr.String(false), err)
	}

	fmt.Printf("found commits: %d\n", len(repoCommits))

	commits := []*Commit{}
	for _, repoCommit := range repoCommits {
		commit, err := GetCommitContent(client, ctx, repoCommit, pr)
		if err != nil {
			return nil, err
		}

		c := &Commit{
			SHA:       commit.GetSHA(),
			Author:    commit.GetCommit().GetAuthor().GetName(),
			Timestamp: commit.GetCommit().GetAuthor().GetDate(),
			Files:     commit.Files,
			Message:   repoCommit.GetCommit().GetMessage(),
		}
		commits = append(commits, c)
	}

	return commits, nil
}

func GetCommitContent(client *github.Client, ctx context.Context, repoCommit *github.RepositoryCommit, pr *PullRequest) (*github.RepositoryCommit, error) {
	sha := repoCommit.GetSHA()
	// fmt.Printf("sha: %s\n", sha)
	commit, _, err := client.Repositories.GetCommit(ctx, pr.Owner, pr.Repo, sha, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commit %s for %s with: %w", sha, pr.String(false), err)
	}

	return commit, nil
}

func getRepoName(url string) string {
	parts := strings.Split(url, "/")
	repo := parts[len(parts)-1]
	return repo
}

func getOwner(url string) string {
	parts := strings.Split(url, "/")
	owner := parts[len(parts)-2]
	return owner
}
