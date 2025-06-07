package gh

import (
	"context"
	"fmt"
	"os"
	"regexp"
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
	ID          int64
	Number      int
	Owner       string
	Repo        string
	Author      string
	Created     github.Timestamp
	Description string
	Title       string
	URL         string
	Commits     []*Commit
	Ticket      string
}

type ReviewsByPullRequest struct {
	PullRequest *PullRequest
	Reviews     []*Review
	Comments    []*github.IssueComment
}

type Review struct {
	// PullRequest *PullRequest
	Summary  *github.PullRequestReview
	Comments []*github.PullRequestComment
}

func (r *Review) String() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("&Review{Summary: %v, ReviewComments: ", r.Summary))
	for _, comment := range r.Comments {
		builder.WriteString(fmt.Sprintf("%v", comment))
	}
	builder.WriteString("}")
	return builder.String()
}

func (rByPR *ReviewsByPullRequest) String() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("&ReviewsByPullRequest{PullRequest: %s, Reviews: ", rByPR.PullRequest.String(false)))
	for _, review := range rByPR.Reviews {
		builder.WriteString(review.String())
	}
	builder.WriteString(", Comments: ")

	for _, comment := range rByPR.Comments {
		builder.WriteString(fmt.Sprintf("%v", comment))
	}
	builder.WriteString("}")
	return builder.String()
}

func (pr *PullRequest) String(verbose bool) string {
	if !verbose {
		return fmt.Sprintf("&PullRequest{Owner: %s, Repo: %s, Author: %s, Created: %s, Title: %s, URL: %s, Ticket: %s",
			pr.Owner, pr.Repo, pr.Author, pr.Created.String(), pr.Title, pr.URL, pr.Ticket)
	}

	var builder strings.Builder
	str := fmt.Sprintf("&PullRequest{Owner: %s, Repo: %s, Author: %s, Created: %s, Description: %s, Title: %s, URL: %s, Ticket: %s Commits: ",
		pr.Owner, pr.Repo, pr.Author, pr.Created.String(), pr.Description, pr.Title, pr.URL, pr.Ticket)
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

func GetPullRequestsByTicket(client *github.Client, ctx context.Context, org, ticket, user, from, to string) ([]*PullRequest, error) {
	opts := &github.SearchOptions{Sort: "created", Order: "desc"}
	queries := []string{
		fmt.Sprintf("org:%s %s type:pr author:%s created:%s..%s", org, ticket, user, from, to),
		fmt.Sprintf("org:%s %s type:pr author:%s updated:%s..%s", org, ticket, user, from, to),
	}

	pullRequests := []*PullRequest{}

	for _, query := range queries {
		fmt.Printf("query: %q\n", query)

		prs, err := GetOrgPullRequestsByQuery(client, ctx, query, opts)
		if err != nil {
			return nil, err
		}

		for _, pr := range prs {
			repo := getRepoName(pr.GetRepositoryURL())
			owner := getOwner(pr.GetRepositoryURL())
			ticket := getTicket(pr.GetTitle())
			pullRequest := &PullRequest{
				ID:          pr.GetID(),
				Number:      pr.GetNumber(),
				Owner:       owner,
				Repo:        repo,
				Author:      pr.GetUser().GetLogin(),
				Created:     pr.GetCreatedAt(),
				Description: pr.GetBody(),
				Title:       pr.GetTitle(),
				URL:         pr.GetURL(),
				Ticket:      ticket,
			}

			commits, err := GetCommitsByPullRequest(client, ctx, pullRequest)
			if err != nil {
				return nil, err
			}
			pullRequest.Commits = commits
			pullRequests = append(pullRequests, pullRequest)
		}
	}

	return pullRequests, nil
}

func GetReviewsByPullRequest(client *github.Client, ctx context.Context, org, repo, user string, prNumber int) ([]*Review, error) {
	GhReviews, err := GetPRReviews(client, ctx, org, repo, prNumber)
	if err != nil {
		return nil, err
	}

	reviews := []*Review{}
	for _, GhReview := range GhReviews {
		if GhReview.GetUser().GetLogin() != user {
			continue
		}

		reviewComments, err := GetPRCommentsByReview(client, ctx, org, repo, prNumber, GhReview.GetID())
		if err != nil {
			return nil, err
		}

		reviews = append(reviews, &Review{Summary: GhReview, Comments: reviewComments})
	}

	return reviews, nil
}

func GetReviewedPullRequests(client *github.Client, ctx context.Context, org, user, from, to string) (map[string]*ReviewsByPullRequest, error) {
	opts := &github.SearchOptions{Sort: "created", Order: "desc"}
	queries := []string{
		fmt.Sprintf("org:%s is:pr commenter:%s -author:%s created:%s..%s", org, user, user, from, to),
		fmt.Sprintf("org:%s is:pr commenter:%s -author:%s updated:%s..%s", org, user, user, from, to),
	}

	reviewsByPR := map[string]*ReviewsByPullRequest{}

	for _, query := range queries {
		fmt.Printf("query: %q\n", query)

		prs, err := GetOrgPullRequestsByQuery(client, ctx, query, opts)
		if err != nil {
			return nil, err
		}

		for _, pr := range prs {
			repo := getRepoName(pr.GetRepositoryURL())
			owner := getOwner(pr.GetRepositoryURL())
			ticket := getTicket(pr.GetTitle())
			pr.GetID()

			pullRequest := &PullRequest{
				ID:          pr.GetID(),
				Number:      pr.GetNumber(),
				Owner:       owner,
				Repo:        repo,
				Author:      pr.GetUser().GetLogin(),
				Created:     pr.GetCreatedAt(),
				Description: pr.GetBody(),
				Title:       pr.GetTitle(),
				URL:         pr.GetURL(),
				Ticket:      ticket,
			}

			GhComments, err := GetPRComments(client, ctx, owner, repo, pr.GetNumber())
			if err != nil {
				return nil, err
			}

			comments := []*github.IssueComment{}
			for _, comment := range GhComments {
				if comment.GetUser().GetLogin() != user {
					continue
				}
				comments = append(comments, comment)
			}

			reviews, err := GetReviewsByPullRequest(client, ctx, owner, repo, user, pr.GetNumber())
			if err != nil {
				return nil, err
			}

			// TODO: check for malformed key with length 0
			key := CreateMapKey(pullRequest.Author, pullRequest.Repo, pullRequest.Number)
			fmt.Printf("key: %s\n", key)

			reviewByPR, exists := reviewsByPR[key]
			if !exists {
				reviewsByPR[key] = &ReviewsByPullRequest{PullRequest: pullRequest, Reviews: reviews, Comments: comments}
			} else {
				// TODO: deduplicate reviewByPR.Reviews and reviewByPR.Comments
				reviewByPR.Reviews = append(reviewByPR.Reviews, reviews...)
				reviewByPR.Comments = append(reviewByPR.Comments, comments...)
			}
		}
	}

	return reviewsByPR, nil
}

func CreateMapKey(owner, repo string, prNum int) string {
	if len(owner) == 0 || len(repo) == 0 || prNum <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/%s/%d", owner, repo, prNum)
}

func ReviewExists(reviewsByPR []*ReviewsByPullRequest, pr *PullRequest) bool {
	for _, review := range reviewsByPR {
		// there's already an entry for the given PR. Update it
		if review.PullRequest.ID == pr.ID {
			return true
		}
	}
	return false
}

func GetPRCommentsByReview(client *github.Client, ctx context.Context, owner, repo string, prNumber int, reviewID int64) ([]*github.PullRequestComment, error) {
	comments, _, err := client.PullRequests.ListReviewComments(ctx, owner, repo, prNumber, reviewID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list comments for review ID %d in PR %d in %s/%s: %w", reviewID, prNumber, owner, repo, err)
	}
	return comments, nil
}

func GetPRComments(client *github.Client, ctx context.Context, owner, repo string, prNumber int) ([]*github.IssueComment, error) {
	comments, _, err := client.Issues.ListComments(ctx, owner, repo, prNumber, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments for PR %d in %s/%s: %w", prNumber, owner, repo, err)
	}
	return comments, nil
}

func GetPRReviews(client *github.Client, ctx context.Context, owner, repo string, prNumber int) ([]*github.PullRequestReview, error) {
	reviews, _, err := client.PullRequests.ListReviews(ctx, owner, repo, prNumber, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments for PR %d in %s/%s: %w", prNumber, owner, repo, err)
	}
	// for i, review := range reviews {
	// 	fmt.Printf("%d: %v\n", i, review)
	// }
	return reviews, nil
}

func GetOrgPullRequestsByQuery(client *github.Client, ctx context.Context, query string, opts *github.SearchOptions) ([]*github.Issue, error) {
	result, _, err := client.Search.Issues(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to search for PRs with query %s: %w", query, err)
	}

	fmt.Printf("total PRs: %d\n", result.GetTotal())

	// for _, res := range result.Issues {
	// 	fmt.Printf("found PR: %s\n", res.String())
	// }
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

func getTicket(PRtitle string) string {
	re := regexp.MustCompile(`[A-Z]{2,}\d{0,}-\d+`)
	match := re.Find([]byte(PRtitle))
	return string(match)
}
