package cmd

import (
	"github.com/spf13/cobra"
)

func newDocsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Google Docs (export via Drive)",
	}
	cmd.AddCommand(newDocsExportCmd(flags))
	return cmd
}

func newDocsExportCmd(flags *rootFlags) *cobra.Command {
	return newExportViaDriveCmd(flags, exportViaDriveOptions{
		Use:           "export <docId>",
		Short:         "Export a Google Doc (pdf|docx|txt)",
		ArgName:       "docId",
		ExpectedMime:  "application/vnd.google-apps.document",
		KindLabel:     "Google Doc",
		DefaultFormat: "pdf",
		FormatHelp:    "Export format: pdf|docx|txt",
	})
}
