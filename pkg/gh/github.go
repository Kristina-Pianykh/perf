package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v72/github"
)

type Commit struct {
	SHA       string
	Author    string
	Timestamp time.Time
	Files     []*CommitFile
	Message   string
}

type CommitFile struct {
	SHA              string `json:"sha,omitempty"`
	Filename         string `json:"filename,omitempty"`
	Status           string `json:"status,omitempty"`
	Patch            string `json:"patch,omitempty"`
	PreviousFilename string `json:"previous_filename,omitempty"`
}

func (c *Commit) isCommitOnDate(date string) (bool, error) {
	targetT, err := parseDate(date)
	if err != nil {
		return false, fmt.Errorf("invalid date string: %w", err)
	}

	commitT := time.Date(c.Timestamp.Year(), c.Timestamp.Month(), c.Timestamp.Day(), 0, 0, 0, 0, time.UTC)
	return commitT.Equal(targetT), nil
}

func (c *Commit) String(withChanges bool) string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
}

type PullRequest struct {
	ID          int64
	Number      int
	Owner       string
	Repo        string
	Author      string
	CreatedAt   time.Time
	Description string
	Title       string
	URL         string
	Commits     []*Commit
	Ticket      string
	Created     bool
	Updated     bool
	Reviewed    bool
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
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data)
}

func (rByPR *ReviewsByPullRequest) String() string {
	data, _ := json.MarshalIndent(rByPR, "", "  ")
	return string(data)
}

func NewCommit(client *github.Client, ctx context.Context, repoCommit *github.RepositoryCommit, pr *PullRequest) (*Commit, error) {
	commit, err := GetCommitContent(client, ctx, repoCommit, pr)
	if err != nil {
		return nil, err
	}

	files := []*CommitFile{}
	for _, f := range commit.Files {
		file := CommitFile{
			SHA:              f.GetSHA(),
			Filename:         f.GetFilename(),
			Status:           f.GetStatus(),
			Patch:            f.GetPatch(),
			PreviousFilename: f.GetPreviousFilename(),
		}
		files = append(files, &file)
	}

	c := Commit{
		SHA:       commit.GetSHA(),
		Author:    commit.GetCommit().GetAuthor().GetName(),
		Timestamp: commit.GetCommit().GetAuthor().GetDate().UTC(),
		Files:     files,
		Message:   repoCommit.GetCommit().GetMessage(),
	}
	return &c, nil
}

func NewPullRequest(
	client *github.Client,
	ctx context.Context,
	pr *github.Issue,
	query, date, ticketID string,
) (*PullRequest, error) {

	repo := getRepoName(pr.GetRepositoryURL())
	owner := getOwner(pr.GetRepositoryURL())

	pullRequest := PullRequest{
		ID:          pr.GetID(),
		Number:      pr.GetNumber(),
		Owner:       owner,
		Repo:        repo,
		Author:      pr.GetUser().GetLogin(),
		CreatedAt:   pr.GetCreatedAt().UTC(),
		Description: pr.GetBody(),
		Title:       pr.GetTitle(),
		URL:         pr.GetURL(),
		Ticket:      ticketID,
	}

	pullRequest.Created = query == "created"
	pullRequest.Updated = query == "updated"
	pullRequest.Reviewed = query == "reviewed"
	return &pullRequest, nil
}

func (pr *PullRequest) FetchCommits(client *github.Client, ctx context.Context, date string) error {
	commits, err := GetCommitsByPullRequest(client, ctx, pr, date)
	if err != nil {
		return err
	}

	pr.Commits = commits
	return nil
}

func (pr *PullRequest) FetchComments(client *github.Client, ctx context.Context) ([]*github.IssueComment, error) {
	comments, _, err := client.Issues.ListComments(ctx, pr.Owner, pr.Repo, pr.Number, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments for PR %d in %s/%s: %w", pr.Number, pr.Owner, pr.Repo, err)
	}
	return comments, nil
}

func (pr *PullRequest) FetchReviews(client *github.Client, ctx context.Context, user string, date string) ([]*Review, error) {
	GhReviews, err := GetPRReviews(client, ctx, pr.Owner, pr.Repo, pr.Number)
	if err != nil {
		return nil, err
	}

	reviews := []*Review{}
	// filter out the reviews that don't belong to the user in question
	for _, GhReview := range GhReviews {
		targetDate, err := parseDate(date)
		if err != nil {
			return nil, err
		}
		if GhReview.GetUser().GetLogin() != user || !GhReview.GetSubmittedAt().UTC().Equal(targetDate) {
			continue
		}

		reviewComments, err := GetPRCommentsByReview(client, ctx, pr.Owner, pr.Repo, pr.Number, GhReview.GetID())
		if err != nil {
			return nil, err
		}

		reviews = append(reviews, &Review{Summary: GhReview, Comments: reviewComments})
	}

	return reviews, nil
}

func (pr *PullRequest) String(verbose bool) string {
	// Marshal the struct to a map first
	type Alias *PullRequest
	base, _ := json.Marshal(Alias(pr))

	var result map[string]interface{}
	json.Unmarshal(base, &result)

	// Conditionally remove fields
	if !verbose {
		delete(result, "Commits")
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return string(data)
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

type Query struct {
	Name  string
	Query string
}

func GetPullRequestsByDate(client *github.Client, ctx context.Context, org, user, from, to string) ([]*PullRequest, error) {
	opts := &github.SearchOptions{Sort: "created", Order: "desc"}
	queries := []Query{
		{
			Name:  "created",
			Query: fmt.Sprintf("org:%s type:pr author:%s created:%s..%s", org, user, from, to),
		}, {
			Name:  "updated",
			Query: fmt.Sprintf("org:%s type:pr author:%s -created:%s..%s updated:%s..%s", org, user, from, to, from, to),
		},
	}

	pullRequests := []*PullRequest{}

	for _, q := range queries {
		fmt.Printf("query: %q\n", q.Name)

		prs, err := GetOrgPullRequestsByQuery(client, ctx, q.Query, opts)
		if err != nil {
			return nil, err
		}

		for _, pr := range prs {

			ticketID := getTicket(pr.GetTitle())
			if len(ticketID) == 0 {
				// no ticket is present in the PR title. Skip processing
				continue
			}

			if alreadyExists(pullRequests, pr.GetID()) {
				continue
			}

			pullRequest, err := NewPullRequest(client, ctx, pr, q.Name, from, ticketID)
			if err != nil {
				return nil, fmt.Errorf("failed to instantiate a *PullRequest: %w", err)
			}
			err = pullRequest.FetchCommits(client, ctx, from)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch commits for PR #%d in %s/%s: %w", pullRequest.Number, pullRequest.Owner, pullRequest.Repo, err)
			}
			pullRequests = append(pullRequests, pullRequest)
		}
	}

	return pullRequests, nil
}

func alreadyExists(prs []*PullRequest, id int64) bool {
	for _, pr := range prs {
		if pr.ID == id {
			return true
		}
	}
	return false
}

func GetReviewsByPullRequest(client *github.Client, ctx context.Context, org, repo, user string, prNumber int, date string) ([]*Review, error) {
	GhReviews, err := GetPRReviews(client, ctx, org, repo, prNumber)
	if err != nil {
		return nil, err
	}

	reviews := []*Review{}
	// filter out the reviews that don't belong to the user in question
	for _, GhReview := range GhReviews {
		targetDate, err := parseDate(date)
		if err != nil {
			return nil, err
		}
		if GhReview.GetUser().GetLogin() != user || !GhReview.GetSubmittedAt().UTC().Equal(targetDate) {
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
	query := Query{
		Name:  "reviewed",
		Query: fmt.Sprintf("org:%s type:pr -author:%s commenter:%s updated:%s..%s", org, user, user, from, to),
	}
	date := from

	reviewsByPR := map[string]*ReviewsByPullRequest{}

	prs, err := GetOrgPullRequestsByQuery(client, ctx, query.Query, opts)
	if err != nil {
		return nil, err
	}

	for _, pr := range prs {
		ticketID := getTicket(pr.GetTitle())

		pullRequest, err := NewPullRequest(client, ctx, pr, query.Name, date, ticketID)
		if err != nil {
			return nil, err
		}

		GhComments, err := pullRequest.FetchComments(client, ctx)
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

		reviews, err := pullRequest.FetchReviews(client, ctx, user, date)
		if err != nil {
			return nil, err
		}

		// TODO: check for malformed key with length 0
		key := CreateMapKey(pullRequest.Author, pullRequest.Repo, pullRequest.Number)
		// fmt.Printf("key: %s\n", key)

		reviewByPR, exists := reviewsByPR[key]
		if !exists {
			reviewsByPR[key] = &ReviewsByPullRequest{PullRequest: pullRequest, Reviews: reviews, Comments: comments}
		} else {
			// TODO: deduplicate reviewByPR.Reviews and reviewByPR.Comments
			reviewByPR.Reviews = append(reviewByPR.Reviews, reviews...)
			reviewByPR.Comments = append(reviewByPR.Comments, comments...)
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

	return result.Issues, nil
}

func GetCommitsByPullRequest(client *github.Client, ctx context.Context, pr *PullRequest, date string) ([]*Commit, error) {
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
		commit, err := NewCommit(client, ctx, repoCommit, pr)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate new commit object of type %T: %w", &Commit{}, err)
		}

		matched, err := commit.isCommitOnDate(date)
		if err != nil {
			return nil, fmt.Errorf("failed to compare commit date with target date: %w", err)
		}

		if matched {
			commits = append(commits, commit)
		}

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

func parseDate(d string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", d)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date string '%s': %w", d, err)
	}
	return date.UTC(), nil
}
