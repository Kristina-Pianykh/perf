package jirautils

import (
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"fmt"

	"github.com/andygrunwald/go-jira"
)

var (
	FilterAlreadyExistsError FilterAlreadyExists
)

type Board struct {
	ToDo       []jira.Issue
	InProgress []jira.Issue
	InReview   []jira.Issue
	Blocked    []jira.Issue
	Done       []jira.Issue
}

type Ticket struct {
	Key      string
	Created  time.Time
	Updated  time.Time
	Assignee string
	Creator  string
	Reporter string
	Title    string
	Body     string
	Status   string
	Comments []*Comment
}

type Comment struct {
	Author    string
	CreatedAt time.Time
	UpdatedAt time.Time
	Body      string
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
	if len(t.Comments) > 0 {
		builder.WriteString("}")
	} else {
		builder.WriteString("}}")
	}
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

func GetIssues(client *jira.Client, filter *jira.Filter) ([]jira.Issue, error) {
	issues, _, err := client.Issue.Search(filter.Jql, nil)
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

func GetProjectId(jiraClient *jira.Client, projectName string) string {
	if jiraClient == nil {
		return ""
	}

	projectId := ""
	projects, _, _ := jiraClient.Project.GetList()
	if projects != nil {
		for _, l := range *projects {
			if l.Name == projectName {
				fmt.Printf("found %s\n", projectName)
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

func DeleteFilter(client *jira.Client, filterID string) error {
	url := "/rest/api/3/filter/" + filterID
	req, err := client.NewRequest("DEL", url, nil)
	if err != nil {
		return fmt.Errorf("failed to prepare a DEL request to %s to delete filter with ID %s: %w", url, filterID, err)
	}

	resp, err := client.Do(req, nil)
	if err != nil {
		return fmt.Errorf("an API error: failed to process DEL request to %s to delete filter with ID %s: %w",
			url, filterID, err)
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("[%d] failed to delete filter with ID %s", resp.StatusCode, filterID)
	}
	return nil
}

func FilterExists(client *jira.Client, filterName string) (*jira.Filter, error) {
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
	filter, err := FilterExists(client, filterName)
	if err != nil {
		return nil, err
	}

	// recreate filter by deleting an existing one with the same name
	if filter != nil {
		// err := DeleteFilter(client, filter.ID)
		// if err != nil {
		// 	return nil, err
		// }
		return filter, nil
	}

	payload := map[string]string{
		"description": "Get all created issues for today",
		"jql":         jql,
		"name":        filterName,
	}
	req, _ := client.NewRequest("POST", "/rest/api/3/filter", payload)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	filter = new(jira.Filter)
	_, err = client.Do(req, filter)
	if err != nil {
		return nil, err
	}
	return filter, nil
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
		jql := fmt.Sprintf("project = DX AND type IN (standardIssueTypes(), subTaskIssueTypes()) AND assignee = currentUser() AND status = \"%s\" ORDER BY created DESC", name)

		filter, err := CreateFilter(client, name, jql)
		if err != nil {
			return nil, fmt.Errorf("failed to create a filter for %s tickets: %w", name, err)
		}
		issues, err := GetIssues(client, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to get issues for filter %s: %s", filter.Name, err.Error())
		}
		slog.Info("", slog.String("filter", filter.Name), slog.Int("ticket", len(issues)))

		for _, issue := range issues {
			slog.Debug("", slog.String("filter", filter.Name), slog.String("ticket", issue.Key))
			jIssue, err := GetIssue(client, issue.Key)
			// raw, _ := json.MarshalIndent(jIssue.Fields, "", "  ")
			// fmt.Println(string(raw))

			if err != nil {
				return nil, err
			}
			ticket, err := newTicket(jIssue)
			if err != nil {
				return nil, err
			}

			allTickets = append(allTickets, ticket)
		}

	}
	slog.Info("", slog.Int("total tickets", len(allTickets)))

	return allTickets, nil
}

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
