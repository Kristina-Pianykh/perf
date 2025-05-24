package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"perf/pkg/jirautils"
)

func initLogger(w io.Writer, minimalLogLevel slog.Level) *slog.Logger {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		// Level: slog.LevelDebug, // Or slog.LevelInfo, slog.LevelError, etc.
		Level: minimalLogLevel,
	})
	return slog.New(handler)
}

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

	_, err = jirautils.GetBoard(jiraClient)
}
