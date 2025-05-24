package jirautils

import (
	"log/slog"
	"net/url"
	"os"

	"fmt"

	"github.com/andygrunwald/go-jira"
)

var (
	FilterAlreadyExistsError FilterAlreadyExists
)

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

	if filter != nil {
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

type Board struct {
	ToDo       []jira.Issue
	InProgress []jira.Issue
	InReview   []jira.Issue
	Blocked    []jira.Issue
	Done       []jira.Issue
}

func GetBoard(client *jira.Client) ([]jira.Issue, error) {
	allTickets := []jira.Issue{}

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
			return nil, fmt.Errorf("failed to create a filter for %s tickets: %s", name, err.Error())
		}
		tickets, err := GetIssues(client, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to get issues for filter %s: %s", filter.Name, err.Error())
		}
		slog.Info("", slog.String("filter", filter.Name), slog.Int("ticket", len(tickets)))

		for _, ticket := range tickets {
			slog.Debug("", slog.String("filter", filter.Name), slog.String("ticket", ticket.Key))
		}

		allTickets = append(allTickets, tickets...)
	}
	slog.Info("", slog.Int("total tickets", len(allTickets)))

	return allTickets, nil
}
