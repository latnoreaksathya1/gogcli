package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type exportViaDriveOptions struct {
	Use           string
	Short         string
	ArgName       string
	ExpectedMime  string
	KindLabel     string
	DefaultFormat string
	FormatHelp    string
}

func newExportViaDriveCmd(flags *rootFlags, opts exportViaDriveOptions) *cobra.Command {
	var outPathFlag string
	var format string

	argName := strings.TrimSpace(opts.ArgName)
	if argName == "" {
		argName = "id"
	}

	cmd := &cobra.Command{
		Use:   opts.Use,
		Short: opts.Short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := ui.FromContext(cmd.Context())
			account, err := requireAccount(flags)
			if err != nil {
				return err
			}

			id := strings.TrimSpace(args[0])
			if id == "" {
				return usage(fmt.Sprintf("empty %s", argName))
			}

			svc, err := newDriveService(cmd.Context(), account)
			if err != nil {
				return err
			}

			meta, err := svc.Files.Get(id).
				SupportsAllDrives(true).
				Fields("id, name, mimeType").
				Context(cmd.Context()).
				Do()
			if err != nil {
				return err
			}
			if meta == nil {
				return errors.New("file not found")
			}
			if opts.ExpectedMime != "" && meta.MimeType != opts.ExpectedMime {
				label := strings.TrimSpace(opts.KindLabel)
				if label == "" {
					label = "expected type"
				}
				return fmt.Errorf("file is not a %s (mimeType=%q)", label, meta.MimeType)
			}

			destPath, err := resolveDriveDownloadDestPath(meta, outPathFlag)
			if err != nil {
				return err
			}

			format = strings.TrimSpace(format)
			if format == "" {
				format = strings.TrimSpace(opts.DefaultFormat)
			}
			if format == "" {
				format = "pdf"
			}

			downloadedPath, size, err := downloadDriveFile(cmd.Context(), svc, meta, destPath, format)
			if err != nil {
				return err
			}

			if outfmt.IsJSON(cmd.Context()) {
				return outfmt.WriteJSON(os.Stdout, map[string]any{"path": downloadedPath, "size": size})
			}
			u.Out().Printf("path\t%s", downloadedPath)
			u.Out().Printf("size\t%s", formatDriveSize(size))
			return nil
		},
	}

	cmd.Flags().StringVar(&outPathFlag, "out", "", "Output file path (default: gogcli config dir)")
	cmd.Flags().StringVar(&format, "format", opts.DefaultFormat, opts.FormatHelp)
	return cmd
}
