package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
	"google.golang.org/api/tasks/v1"
)

func newTasksListsCmd(flags *rootFlags) *cobra.Command {
	var max int64
	var page string

	cmd := &cobra.Command{
		Use:   "lists",
		Short: "List task lists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := ui.FromContext(cmd.Context())
			account, err := requireAccount(flags)
			if err != nil {
				return err
			}

			svc, err := newTasksService(cmd.Context(), account)
			if err != nil {
				return err
			}

			call := svc.Tasklists.List().MaxResults(max).PageToken(page)
			resp, err := call.Do()
			if err != nil {
				return err
			}

			if outfmt.IsJSON(cmd.Context()) {
				return outfmt.WriteJSON(os.Stdout, map[string]any{
					"tasklists":     resp.Items,
					"nextPageToken": resp.NextPageToken,
				})
			}

			if len(resp.Items) == 0 {
				u.Err().Println("No task lists")
				return nil
			}

			w, flush := tableWriter(cmd.Context())
			defer flush()
			fmt.Fprintln(w, "ID\tTITLE")
			for _, tl := range resp.Items {
				fmt.Fprintf(w, "%s\t%s\n", tl.Id, tl.Title)
			}
			printNextPageHint(u, resp.NextPageToken)
			return nil
		},
	}

	cmd.Flags().Int64Var(&max, "max", 100, "Max results (max allowed: 1000)")
	cmd.Flags().StringVar(&page, "page", "", "Page token")
	cmd.AddCommand(newTasksListsCreateCmd(flags))
	return cmd
}

func newTasksListsCreateCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create <title>",
		Short:   "Create a task list",
		Aliases: []string{"add", "new"},
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := ui.FromContext(cmd.Context())
			account, err := requireAccount(flags)
			if err != nil {
				return err
			}
			title := strings.TrimSpace(strings.Join(args, " "))
			if title == "" {
				return usage("empty title")
			}

			svc, err := newTasksService(cmd.Context(), account)
			if err != nil {
				return err
			}

			created, err := svc.Tasklists.Insert(&tasks.TaskList{Title: title}).Do()
			if err != nil {
				return err
			}

			if outfmt.IsJSON(cmd.Context()) {
				return outfmt.WriteJSON(os.Stdout, map[string]any{"tasklist": created})
			}
			u.Out().Printf("id\t%s", created.Id)
			u.Out().Printf("title\t%s", created.Title)
			return nil
		},
	}
	return cmd
}
