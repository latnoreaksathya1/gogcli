package cmd

import (
	"context"
	"fmt"
	"net/mail"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
	"google.golang.org/api/gmail/v1"
)

var newGmailService = googleapi.NewGmail

func newGmailCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gmail",
		Short: "Gmail",
	}
	cmd.AddCommand(newGmailSearchCmd(flags))
	cmd.AddCommand(newGmailThreadCmd(flags))
	cmd.AddCommand(newGmailGetCmd(flags))
	cmd.AddCommand(newGmailAttachmentCmd(flags))
	cmd.AddCommand(newGmailURLCmd(flags))
	cmd.AddCommand(newGmailLabelsCmd(flags))
	cmd.AddCommand(newGmailSendCmd(flags))
	cmd.AddCommand(newGmailDraftsCmd(flags))
	cmd.AddCommand(newGmailWatchCmd(flags))
	cmd.AddCommand(newGmailHistoryCmd(flags))
	cmd.AddCommand(newGmailAutoForwardCmd(flags))
	cmd.AddCommand(newGmailBatchCmd(flags))
	cmd.AddCommand(newGmailDelegatesCmd(flags))
	cmd.AddCommand(newGmailFiltersCmd(flags))
	cmd.AddCommand(newGmailForwardingCmd(flags))
	cmd.AddCommand(newGmailSendAsCmd(flags))
	cmd.AddCommand(newGmailVacationCmd(flags))
	return cmd
}

func newGmailSearchCmd(flags *rootFlags) *cobra.Command {
	var max int64
	var page string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search threads using Gmail query syntax",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := ui.FromContext(cmd.Context())
			account, err := requireAccount(flags)
			if err != nil {
				return err
			}
			query := strings.Join(args, " ")

			svc, err := newGmailService(cmd.Context(), account)
			if err != nil {
				return err
			}

			resp, err := svc.Users.Threads.List("me").
				Q(query).
				MaxResults(max).
				PageToken(page).
				Context(cmd.Context()).
				Do()
			if err != nil {
				return err
			}

			idToName, err := fetchLabelIDToName(svc)
			if err != nil {
				return err
			}

			// Fetch thread details concurrently (fixes N+1 query pattern)
			items, err := fetchThreadDetails(cmd.Context(), svc, resp.Threads, idToName)
			if err != nil {
				return err
			}

			if outfmt.IsJSON(cmd.Context()) {
				return outfmt.WriteJSON(os.Stdout, map[string]any{
					"threads":       items,
					"nextPageToken": resp.NextPageToken,
				})
			}

			if len(items) == 0 {
				u.Err().Println("No results")
				return nil
			}

			w, flush := tableWriter(cmd.Context())
			defer flush()

			fmt.Fprintln(w, "ID\tDATE\tFROM\tSUBJECT\tLABELS")
			for _, it := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", it.ID, it.Date, it.From, it.Subject, strings.Join(it.Labels, ","))
			}
			printNextPageHint(u, resp.NextPageToken)
			return nil
		},
	}

	cmd.Flags().Int64Var(&max, "max", 10, "Max results")
	cmd.Flags().StringVar(&page, "page", "", "Page token")
	return cmd
}

func firstMessage(t *gmail.Thread) *gmail.Message {
	if t == nil || len(t.Messages) == 0 {
		return nil
	}
	return t.Messages[0]
}

func headerValue(p *gmail.MessagePart, name string) string {
	if p == nil {
		return ""
	}
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func formatGmailDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if t, err := mailParseDate(raw); err == nil {
		return t.Format("2006-01-02 15:04")
	}
	return raw
}

func sanitizeTab(s string) string {
	return strings.ReplaceAll(s, "\t", " ")
}

func mailParseDate(s string) (time.Time, error) {
	// net/mail has the most compatible Date parser, but we keep this isolated for easier tests/mocks later.
	return mail.ParseDate(s)
}

// threadItem holds parsed thread metadata for display/JSON output
type threadItem struct {
	ID      string   `json:"id"`
	Date    string   `json:"date,omitempty"`
	From    string   `json:"from,omitempty"`
	Subject string   `json:"subject,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}

// fetchThreadDetails fetches thread metadata concurrently with bounded parallelism.
// This eliminates N+1 queries by fetching all threads in parallel.
func fetchThreadDetails(ctx context.Context, svc *gmail.Service, threads []*gmail.Thread, idToName map[string]string) ([]threadItem, error) {
	if len(threads) == 0 {
		return nil, nil
	}

	const maxConcurrency = 10 // Limit parallel requests to avoid rate limiting
	sem := make(chan struct{}, maxConcurrency)

	type result struct {
		index int
		item  threadItem
		err   error
	}

	results := make(chan result, len(threads))
	var wg sync.WaitGroup

	for i, t := range threads {
		if t.Id == "" {
			continue
		}

		wg.Add(1)
		go func(idx int, threadID string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{index: idx, err: ctx.Err()}
				return
			}

			thread, err := svc.Users.Threads.Get("me", threadID).
				Format("metadata").
				MetadataHeaders("From", "Subject", "Date").
				Context(ctx).
				Do()
			if err != nil {
				results <- result{index: idx, err: err}
				return
			}

			item := threadItem{ID: threadID}
			if msg := firstMessage(thread); msg != nil {
				item.Date = formatGmailDate(headerValue(msg.Payload, "Date"))
				item.From = sanitizeTab(headerValue(msg.Payload, "From"))
				item.Subject = sanitizeTab(headerValue(msg.Payload, "Subject"))
				if len(msg.LabelIds) > 0 {
					names := make([]string, 0, len(msg.LabelIds))
					for _, id := range msg.LabelIds {
						if n, ok := idToName[id]; ok {
							names = append(names, n)
						} else {
							names = append(names, id)
						}
					}
					item.Labels = names
				}
			}

			results <- result{index: idx, item: item}
		}(i, t.Id)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results in order
	items := make([]threadItem, len(threads))
	validCount := 0
	for r := range results {
		if r.err != nil {
			return nil, r.err
		}
		items[r.index] = r.item
		validCount++
	}

	// Filter out empty items (from threads with empty IDs)
	filtered := make([]threadItem, 0, validCount)
	for _, item := range items {
		if item.ID != "" {
			filtered = append(filtered, item)
		}
	}

	return filtered, nil
}
