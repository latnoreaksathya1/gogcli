package cmd

import (
	"context"
	"io"
	"os"
	"text/tabwriter"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func tableWriter(ctx context.Context) (io.Writer, func()) {
	if outfmt.IsPlain(ctx) {
		return os.Stdout, func() {}
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	return tw, func() { _ = tw.Flush() }
}

func printNextPageHint(u *ui.UI, nextPageToken string) {
	if u == nil || nextPageToken == "" {
		return
	}
	u.Err().Printf("# Next page: --page %s", nextPageToken)
}
