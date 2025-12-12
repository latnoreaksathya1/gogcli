package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestExecute_DriveGet_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// google.golang.org/api/drive sometimes uses basepaths with or without /drive/v3.
		// For this test we accept any GET and return the metadata payload.
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "id1",
			"name":         "Doc",
			"mimeType":     "application/pdf",
			"size":         "1024",
			"modifiedTime": "2025-12-12T14:37:47Z",
			"createdTime":  "2025-12-11T00:00:00Z",
			"starred":      true,
		})
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--output", "json", "--account", "a@b.com", "drive", "get", "id1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		File struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Starred bool   `json:"starred"`
		} `json:"file"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.File.ID != "id1" || parsed.File.Name != "Doc" || !parsed.File.Starred {
		t.Fatalf("unexpected file: %#v", parsed.File)
	}
}

func TestExecute_DriveDownload_JSON(t *testing.T) {
	origNew := newDriveService
	origDownload := driveDownload
	t.Cleanup(func() {
		newDriveService = origNew
		driveDownload = origDownload
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Metadata fetch (Do()).
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "file.bin",
			"mimeType": "application/pdf",
		})
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	driveDownload = func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("abc")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "out.bin")
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--output", "json", "--account", "a@b.com", "drive", "download", "id1", dest}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Path != dest || parsed.Size != 3 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if b, err := os.ReadFile(dest); err != nil || string(b) != "abc" {
		t.Fatalf("file mismatch: err=%v body=%q", err, string(b))
	}

	// Sanity: downloads dir is still creatable (but we passed dest explicitly).
	if _, err := config.EnsureDriveDownloadsDir(); err != nil {
		t.Fatalf("EnsureDriveDownloadsDir: %v", err)
	}
}

func TestDriveDownloadCmd_FileHasNoName(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "",
			"mimeType": "application/pdf",
		})
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &rootFlags{Account: "a@b.com"}
	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.ModeText)

	cmd := newDriveDownloadCmd(flags)
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"id1", filepath.Join(t.TempDir(), "out.bin")})
	if execErr := cmd.Execute(); execErr == nil || !strings.Contains(execErr.Error(), "file has no name") {
		t.Fatalf("expected file has no name error, got: %v", execErr)
	}
}
