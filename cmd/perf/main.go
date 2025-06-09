package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"perf/pkg/gh"
	"perf/pkg/jirautils"
	"perf/pkg/openai"
	"strings"
	"time"
)

func initLogger(w io.Writer, minimalLogLevel slog.Level) *slog.Logger {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		// Level: slog.LevelDebug, // Or slog.LevelInfo, slog.LevelError, etc.
		Level: minimalLogLevel,
	})
	return slog.New(handler)
}

const (
	ORG      = "goflink"
	USERNAME = "Kristina-Pianykh"
)

func main() {
	out := os.Stdout
	logger := initLogger(out, slog.LevelDebug)
	slog.SetDefault(logger)

	jiraClient, err := jirautils.InitJiraClient()
	if err != nil {
		fmt.Fprintf(out, "failed to create a Jira client: %s", err.Error())
		return
	}

	project := "Developer Experience"
	projectId := jirautils.GetProjectId(jiraClient, project)
	fmt.Printf("%s project has ID: %s\n", project, projectId)

	// updateFilter, err := jirautils.CreateFilter(
	// 	jiraClient,
	// 	"Get all updated issues for today",
	// 	"project = DX AND type IN (standardIssueTypes(), subTaskIssueTypes()) AND assignee = currentUser() AND updated >= \"2025-05-24\" AND updated <= \"2025-05-25\" ORDER BY created DESC",
	// )
	// if err != nil {
	// 	slog.Error("failed to create a filter for updated tickets: %s", err.Error())
	// }
	// // fmt.Printf("created an update filter: %v\n", *updateFilter)
	//
	// createFilter, err := jirautils.CreateFilter(
	// 	jiraClient,
	// 	"created issues for today",
	// 	"project = DX AND type IN (standardIssueTypes(), subTaskIssueTypes()) AND created >= \"2025-05-24\" AND created <= \"2025-05-25\" AND reporter = currentUser() ORDER BY created DESC",
	// )
	// if err != nil {
	// 	slog.Error("failed to create a filter for created issues: %s", err.Error())
	// }
	// // fmt.Printf("\ncreated a create filter: %v\n", *createFilter)
	//
	// updatedIssues, err := jirautils.GetIssues(jiraClient, updateFilter)
	// createdIssues, err := jirautils.GetIssues(jiraClient, createFilter)
	// fmt.Printf("updated issues: %d\n", len(updatedIssues))
	// fmt.Printf("created issues: %d\n", len(createdIssues))

	// tickets, err := jirautils.GetBoard(jiraClient)
	// if err != nil {
	// 	fmt.Printf("%s", err.Error())
	// }
	//
	// for _, ticket := range tickets {
	// 	fmt.Printf("Ticket: %s\n\n\n", ticket.String())
	// }

	ghClient, err := gh.InitClient()
	if err != nil {
		fmt.Fprintf(out, "failed to create a GitHub client: %s", err.Error())
		return
	}

	from := "2025-06-05"
	to := "2025-06-07"
	ctx := context.Background()
	prs, err := gh.GetPullRequestsByDate(ghClient, ctx, ORG, USERNAME, from, to)
	if err != nil {
		log.Fatal(err)
	}

	relevantTickets, err := jirautils.AggPullRequestsByTicket(jiraClient, prs)
	if err != nil {
		log.Fatal(err)
	}

	var inputBuilder strings.Builder
	inputBuilder.WriteString("Individual contributions by Jira Ticket\n")
	for key, ticket := range relevantTickets {
		inputBuilder.WriteString(fmt.Sprintf("TICKET [%s]: %s\n\n", key, ticket))
	}

	reviewsByPR, err := gh.GetReviewedPullRequests(ghClient, ctx, ORG, USERNAME, from, to)
	if err != nil {
		log.Fatal(err)
	}

	inputBuilder.WriteString("\n\nReveiwed Pull Requests\n")
	for _, reviewByPR := range reviewsByPR {
		inputBuilder.WriteString(reviewByPR.String())
	}

	aiClient, err := openai.InitClient()
	if err != nil {
		log.Fatal(err)
	}
	input := inputBuilder.String()
	output, err := openai.Complete(aiClient, context.Background(), &input)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(*output)
}

func today() time.Time {
	now := time.Now()
	midnight := time.Date(
		now.Year(), now.Month(), now.Day(),
		0, 0, 0, 0,
		now.Location(), // or time.UTC if you want UTC midnight
	)
	return midnight
}

func yesterday() time.Time {
	now := time.Now()
	yesterday := time.Date(
		now.Year(), now.Month(), now.Day()-1,
		0, 0, 0, 0,
		now.Location(), // or time.UTC
	)
	return yesterday
}
