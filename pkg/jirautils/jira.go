package jirautils

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/url"
	"os"
	"perf/pkg/gh"
	"strings"
	"time"

	"fmt"

	"github.com/andygrunwald/go-jira"
)

var (
	FilterAlreadyExistsError FilterAlreadyExists
)

type Filter struct {
	Name string
	Jql  string
}

type Board struct {
	ToDo       []jira.Issue
	InProgress []jira.Issue
	InReview   []jira.Issue
	Blocked    []jira.Issue
	Done       []jira.Issue
}

type Ticket struct {
	Key          string
	Created      time.Time
	Updated      time.Time
	Assignee     string
	Creator      string
	Reporter     string
	Title        string
	Body         string
	Status       string
	PullRequests []*gh.PullRequest
	Comments     []*Comment
}

type Comment struct {
	Author    string
	CreatedAt time.Time
	UpdatedAt time.Time
	Body      string
}

type ChangelogItem struct {
	Ticket  *Ticket
	Changes []*Change
}

type Change struct {
	Field string
	From  string
	To    string
}

func (c *Change) String() string {
	return fmt.Sprintf("&Change{%s changed from %s to %s}", c.Field, c.From, c.To)
}

func (cli *ChangelogItem) String() string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("&ChangelogItem:{Ticket: %s, Changes: ", cli.Ticket.String()))

	if len(cli.Changes) == 0 {
		builder.WriteString("{}")
	} else {
		for _, change := range cli.Changes {
			builder.WriteString(change.String())
			builder.WriteString(",")
		}
	}
	builder.WriteString("}")
	return builder.String()
}

// TODO: unit test?
func (t *Ticket) AddPullRequest(pr *gh.PullRequest) {
	if t.PullRequests == nil {
		t.PullRequests = []*gh.PullRequest{pr}
		return
	}

	if len(t.PullRequests) == 0 {
		t.PullRequests = append(t.PullRequests, pr)
		return
	}

	for _, existingPr := range t.PullRequests {
		if existingPr.ID == pr.ID {
			return
		}
	}
	t.PullRequests = append(t.PullRequests, pr)
}

func (t *Ticket) String() string {
	var builder strings.Builder
	builder.WriteString("&Ticket{Key: " + t.Key)
	builder.WriteString(fmt.Sprintf(", Created: %v,", t.Created))
	builder.WriteString(fmt.Sprintf("Updated: %v,", t.Updated))
	builder.WriteString("Assignee: " + t.Assignee)
	builder.WriteString(", Creator: " + t.Creator)
	builder.WriteString(", Reporter: " + t.Reporter)
	builder.WriteString(", Title: \"" + t.Title + "\"")
	builder.WriteString(", Status: " + t.Status)
	builder.WriteString(", Body: \"" + t.Body + "\"")
	builder.WriteString(", Comments: {")
	for _, comment := range t.Comments {
		builder.WriteString(fmt.Sprintf("%s,", comment.String()))
	}

	builder.WriteString("}, &PullRequests: {")
	for _, pr := range t.PullRequests {
		builder.WriteString(pr.String(true))
	}
	builder.WriteString("}")
	return builder.String()
}

func (c *Comment) String() string {
	return fmt.Sprintf("&Comment{Author: %s, CreatedAt: %v, UpdatedAt: %v, Body: %s}", c.Author, c.CreatedAt, c.UpdatedAt, c.Body)
}

type FilterAlreadyExists struct {
	name string
}

func NewFilterAlreadyExistsError(name string) FilterAlreadyExists {
	return FilterAlreadyExists{name: name}
}

func (e FilterAlreadyExists) Error() string {
	return fmt.Sprintf("filter with name '%s' already exists", e.name)
}

func GetIssues(client *jira.Client, filter *jira.Filter, opts *jira.SearchOptions) ([]jira.Issue, error) {
	issues, _, err := client.Issue.Search(filter.Jql, opts)
	if err != nil {
		return nil, err
	}
	if issues != nil {
		fmt.Printf("found %d issues for filter '%s'\n", len(issues), filter.Name)
	}
	for _, is := range issues {
		fmt.Printf("ID: %s\n", is.Key)
	}
	return issues, nil
}

func GetIssue(client *jira.Client, key string) (*jira.Issue, error) {
	// TODO: specify options for specific fields, by default it pulls all of them
	opts := &jira.GetQueryOptions{
		Fields: "assignee,creator,reporter,summary,description,comment,created,updated,status",
	}
	issue, _, err := client.Issue.Get(key, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ticket with key %s: %w", key, err)
	}
	return issue, nil
}

func GetTicketByKey(client *jira.Client, key string) (*Ticket, error) {
	jIssue, err := GetIssue(client, key)
	if err != nil {
		return nil, err
	}

	ticket, err := newTicket(jIssue)
	if err != nil {
		return nil, err
	}
	return ticket, nil
}

func GetProjectId(jiraClient *jira.Client, projectName string) string {
	if jiraClient == nil {
		return ""
	}

	projectId := ""
	projects, _, _ := jiraClient.Project.GetList()
	if projects != nil {
		for _, l := range *projects {
			if l.Name == projectName {
				projectId = l.ID
				break
			}
		}
	}
	return projectId
}

func InitJiraClient() (*jira.Client, error) {
	domain := "https://goflink.atlassian.net"
	token, ok := os.LookupEnv("JIRA_API_TOKEN")
	if !ok {
		return nil, fmt.Errorf("missing JIRA_API_TOKEN")
	}

	username, ok := os.LookupEnv("JIRA_USERNAME")
	if !ok {
		return nil, fmt.Errorf("missing JIRA_USERNAME")
	}
	tp := jira.BasicAuthTransport{
		Username: username,
		Password: token,
	}
	client, err := jira.NewClient(tp.Client(), domain)
	return client, err
}

func UpdateFilter(client *jira.Client, filter *jira.Filter, Jql string) error {
	filterID := filter.ID

	url := "/rest/api/3/filter/" + filterID
	body := map[string]string{
		"jql":  Jql,
		"name": filter.Name,
	}
	data, _ := json.Marshal(body)

	req, err := client.NewRequest("PUT", url, body)
	if err != nil {
		return fmt.Errorf("failed to prepare a PUT request to %s to update filter with ID %s: %w", url, filterID, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	fmt.Printf("payload for PUT: %s\n", data)
	fmt.Printf("Host for PUT: %s\n", req.URL.Host)
	fmt.Printf("path for PUT: %s\n", req.URL.Path)
	fmt.Printf("headers for PUT: %v\n", req.Header)

	resp, err := client.Do(req, nil)
	if err != nil {
		return fmt.Errorf("an API error: failed to process PUT request to %s to delete filter with ID %s: %w",
			url, filterID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("[%d] failed to update filter with ID %s", resp.StatusCode, filterID)
	}
	return nil
}

func GetFilter(client *jira.Client, filterName string) (*jira.Filter, error) {
	var result struct {
		Total   int            `json:"total"`
		Filters []*jira.Filter `json:"values"`
	}

	values := url.Values{}
	// values.Set("accountId", "712020")
	values.Set("filterName", filterName)
	values.Set("expand", "jql,owner") // Ensure expanded fields
	params := values.Encode()

	req, _ := client.NewRequest("GET", "/rest/api/3/filter/search?"+params, nil)

	fmt.Println(req.URL.String())
	_, err := client.Do(req, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to check if filter with name '%s' already exists: %s",
			filterName, err.Error())
	}

	fmt.Printf("got filters\n")
	for _, filter := range result.Filters {
		// fmt.Printf("filter: %s\n", filter.Name)
		if filter.Name == filterName {
			fmt.Printf("this is our filter\n")
			fmt.Printf("Jql: %s\n", filter.Jql)
			return filter, nil
		}
	}
	return nil, nil
}

func CreateFilter(client *jira.Client, filterName, jql string) (*jira.Filter, error) {
	filter, err := GetFilter(client, filterName)
	if err != nil {
		return nil, err
	}

	if filter != nil {
		err := UpdateFilter(client, filter, jql)
		if err != nil {
			return nil, err
		}
		return filter, nil
	}

	payload := map[string]string{
		"jql":  jql,
		"name": filterName,
	}
	req, _ := client.NewRequest("POST", "/rest/api/3/filter", payload)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	filter = new(jira.Filter)
	// fmt.Printf("req: %v\n", req)
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(payload)
	fmt.Println("JSON payload:", buf.String())

	_, err = client.Do(req, filter)
	if err != nil {
		return nil, err
	}
	return filter, nil
}

func GetTicketsByFilter(client *jira.Client, filter *Filter) ([]*Ticket, error) {
	jFilter, err := CreateFilter(client, filter.Name, filter.Jql)
	if err != nil {
		return nil, fmt.Errorf("failed to create a filter for %s tickets: %w", filter.Name, err)
	}
	issues, err := GetIssues(client, jFilter, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get issues for filter %s: %s", jFilter.Name, err.Error())
	}
	// slog.Info("", slog.String("filter", jFilter.Name), slog.Int("ticket", len(issues)))

	allTickets := []*Ticket{}
	for _, issue := range issues {
		// slog.Debug("", slog.String("filter", jFilter.Name), slog.String("ticket", issue.Key))

		ticket, err := GetTicketByKey(client, issue.Key)
		if err != nil {
			return nil, err
		}

		allTickets = append(allTickets, ticket)
	}
	return allTickets, nil
}

func GetBoard(client *jira.Client) ([]*Ticket, error) {
	allTickets := []*Ticket{}

	filterNames := []string{
		"Selected for Development",
		"In Progress",
		"In Review",
		"Blocked",
		"Canceled",
		"Done",
		"BACKLOG",
	}

	for _, name := range filterNames {
		f := Filter{
			Name: name,
			Jql:  fmt.Sprintf("project = DX AND type IN (standardIssueTypes(), subTaskIssueTypes()) AND assignee = currentUser() AND status = \"%s\" ORDER BY created DESC", name),
		}

		tickets, err := GetTicketsByFilter(client, &f)
		if err != nil {
			return nil, err
		}

		allTickets = append(allTickets, tickets...)
	}

	slog.Info("", slog.Int("total tickets", len(allTickets)))

	return allTickets, nil
}

// func GetUpdatedTickets(client *jira.Client, from, to, user string) ([]*ChangelogItem, error) {
// 	jql := fmt.Sprintf("project = DX AND updated >= \"%s\" AND updated < \"%s\" AND assignee = \"Kristina Pianykh\"", from, to)
// 	fmt.Printf("jql: %s\n", jql)
// 	filterName := "updatedToday"
//
// 	filter, err := CreateFilter(client, filterName, jql)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create a filter for %s tickets: %w", filterName, err)
// 	}
// 	issues, err := GetIssues(client, filter, &jira.SearchOptions{Expand: "changelog"})
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get issues for filter %s: %s", filter.Name, err.Error())
// 	}
// 	// slog.Info("", slog.String("filter", filter.Name), slog.Int("ticket", len(issues)))
//
// 	changelog := []*ChangelogItem{}
//
// 	for _, issue := range issues {
// 		// slog.Debug("", slog.String("filter", filter.Name), slog.String("ticket", issue.Key))
// 		// fmt.Printf("%v\n", issue.Changelog.Histories)
//
// 		if len(issue.Changelog.Histories) == 0 {
// 			continue
// 		}
//
// 		ticket, err := GetTicketByKey(client, issue.Key)
// 		if err != nil {
// 			return nil, err
// 		}
//
// 		changes := []*Change{}
//
// 		for _, history := range (*issue.Changelog).Histories {
// 			for _, item := range history.Items {
//
// 				if strings.ToLower(item.Field) == "description" && history.Author.DisplayName != user {
// 					continue
// 				}
//
// 				if slices.Contains([]string{"rank", "link", "fix version", "issueparentassociation", "assignee", "remoteissuelink", "labels"}, strings.ToLower(item.Field)) {
// 					continue
// 				}
//
// 				// fmt.Printf("%v\n", item)
// 				change := Change{Field: item.Field, From: item.FromString, To: item.ToString}
// 				changes = append(changes, &change)
// 			}
// 		}
//
// 		changelogItem := ChangelogItem{Ticket: ticket, Changes: changes}
// 		changelog = append(changelog, &changelogItem)
// 	}
// 	return changelog, nil
// }

func newTicket(jIssue *jira.Issue) (*Ticket, error) {
	ticket := Ticket{
		Key:      jIssue.Key,
		Created:  time.Time(jIssue.Fields.Created),
		Updated:  time.Time(jIssue.Fields.Updated),
		Assignee: jIssue.Fields.Assignee.DisplayName,
		Creator:  jIssue.Fields.Creator.DisplayName,
		Reporter: jIssue.Fields.Reporter.DisplayName,
		Title:    jIssue.Fields.Summary,
		Status:   jIssue.Fields.Status.Name,
		Body:     jIssue.Fields.Description,
	}

	if jIssue.Fields.Comments != nil {
		if len(jIssue.Fields.Comments.Comments) > 0 {
			comments := []*Comment{}
			for _, c := range jIssue.Fields.Comments.Comments {
				comment, err := newComment(c)
				if err != nil {
					return nil, err
				}
				comments = append(comments, comment)
			}
			ticket.Comments = comments
		}
	}
	return &ticket, nil
}

func newComment(c *jira.Comment) (*Comment, error) {
	createdAt, err := parseJiraTime(c.Created)
	if err != nil {
		return nil, err
	}
	updatedAt, err := parseJiraTime(c.Updated)
	if err != nil {
		return nil, err
	}

	comment := Comment{
		Author:    c.Author.Name,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Body:      c.Body,
	}
	return &comment, nil
}

func parseJiraTime(t string) (time.Time, error) {
	const jiraLayout = "2006-01-02T15:04:05.000-0700"
	parsed, err := time.Parse(jiraLayout, t)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse %s into time.Time: %w", t, err)
	}
	return parsed, nil
}

func AggPullRequestsByTicket(client *jira.Client, prs []*gh.PullRequest) (map[string]*Ticket, error) {
	relevantTickets := map[string]*Ticket{}

	for _, pr := range prs {
		if ticket, exists := relevantTickets[pr.Ticket]; exists {
			ticket.AddPullRequest(pr)
			continue
		}

		ticket, err := GetTicketByKey(client, pr.Ticket)
		if err != nil {
			return nil, err
		}
		ticket.AddPullRequest(pr)
		relevantTickets[pr.Ticket] = ticket
	}
	return relevantTickets, nil
}
