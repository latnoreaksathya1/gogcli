package cmd

import (
	"github.com/spf13/cobra"
)

func newSlidesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "slides",
		Short: "Google Slides (export via Drive)",
	}
	cmd.AddCommand(newSlidesExportCmd(flags))
	return cmd
}

func newSlidesExportCmd(flags *rootFlags) *cobra.Command {
	return newExportViaDriveCmd(flags, exportViaDriveOptions{
		Use:           "export <presentationId>",
		Short:         "Export a Google Slides deck (pdf|pptx)",
		ArgName:       "presentationId",
		ExpectedMime:  "application/vnd.google-apps.presentation",
		KindLabel:     "Google Slides presentation",
		DefaultFormat: "pptx",
		FormatHelp:    "Export format: pdf|pptx",
	})
}
