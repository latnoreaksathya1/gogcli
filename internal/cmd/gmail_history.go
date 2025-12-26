package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newGmailHistoryCmd(flags *rootFlags) *cobra.Command {
	var since string
	var max int64
	var page string

	cmd := &cobra.Command{
		Use:   "history",
		Short: "List Gmail history entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := ui.FromContext(cmd.Context())
			account, err := requireAccount(flags)
			if err != nil {
				return err
			}
			if strings.TrimSpace(since) == "" {
				return usage("--since is required")
			}
			startID, err := parseHistoryID(since)
			if err != nil {
				return err
			}

			svc, err := newGmailService(cmd.Context(), account)
			if err != nil {
				return err
			}

			call := svc.Users.History.List("me").StartHistoryId(startID).MaxResults(max)
			call.HistoryTypes("messageAdded")
			if strings.TrimSpace(page) != "" {
				call.PageToken(page)
			}
			resp, err := call.Do()
			if err != nil {
				return err
			}

			ids := collectHistoryMessageIDs(resp)
			if outfmt.IsJSON(cmd.Context()) {
				return outfmt.WriteJSON(os.Stdout, map[string]any{
					"historyId":     formatHistoryID(resp.HistoryId),
					"messages":      ids,
					"nextPageToken": resp.NextPageToken,
				})
			}
			if len(ids) == 0 {
				u.Err().Println("No history")
				return nil
			}
			u.Out().Println("MESSAGE_ID")
			for _, id := range ids {
				u.Out().Println(id)
			}
			printNextPageHint(u, resp.NextPageToken)
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "", "Start history ID")
	cmd.Flags().Int64Var(&max, "max", defaultHistoryMaxResults, "Max results")
	cmd.Flags().StringVar(&page, "page", "", "Page token")
	return cmd
}
