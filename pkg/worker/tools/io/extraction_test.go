package io

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/apps/worker/pkg/worker/sourcefiles"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository/mock"
	"github.com/synthify/backend/packages/shared/storage"
)

func TestExtractionTool_Zip(t *testing.T) {
	// 1. Setup Mock FUSE
	tmpDir := t.TempDir()
	sourcefiles.FUSE = storage.NewFUSEHandler(tmpDir)
	defer func() { sourcefiles.FUSE = nil }()

	// 2. Setup Mock Repo
	store := mock.NewStore()

	// 3. Create Mock Zip Content
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	
	files := []struct {
		Name, Body string
	}{
		{"readme.txt", "This is a readme."},
		{"src/main.go", "package main\nfunc main() {}"},
		{"docs/nested.md", "# Nested Doc"},
	}
	for _, file := range files {
		f, err := zw.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write([]byte(file.Body))
		if err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()

	// 4. Setup Context
	wsID := "ws_1"
	docID := "doc_zip"
	b := &base.Context{
		Repo: store,
		Job: &base.JobContext{
			WorkspaceID: wsID,
			DocumentID:  docID,
		},
	}

	t.Run("Extract ZIP and verify files", func(t *testing.T) {
		source := domain.SourceFile{
			Filename:    "test.zip",
			URI:         "mock://zip",
			MimeType:    "application/zip",
			Content:     buf.Bytes(),
			WorkspaceID: wsID,
			DocumentID:  docID,
		}

		res, err := processZip(context.Background(), b, source)
		require.NoError(t, err)

		// Verify combined text output (with new ID markers)
		if !strings.Contains(res.RawText, "--- File: readme.txt (ID: file-readme.txt) ---") {
			t.Errorf("missing readme marker, got: %q", res.RawText)
		}
		if !strings.Contains(res.RawText, "package main") {
			t.Errorf("missing go content")
		}

		// Verify files on FUSE mount
		extractPath := filepath.Join(tmpDir, wsID, docID)
		if _, err := os.Stat(filepath.Join(extractPath, "src/main.go")); err != nil {
			t.Errorf("file not extracted to FUSE: %v", err)
		}

		// Verify DB entries
		docFiles, _ := store.ListDocumentFiles(context.Background(), docID)
		if len(docFiles) != 3 {
			t.Errorf("expected 3 DB file records, got %d", len(docFiles))
		}
	})
}

func TestExtractionTool_SingleFile(t *testing.T) {
	// 1. Setup Mock FUSE
	tmpDir := t.TempDir()
	sourcefiles.FUSE = storage.NewFUSEHandler(tmpDir)
	defer func() { sourcefiles.FUSE = nil }()

	// 2. Setup Mock Repo
	store := mock.NewStore()

	// 3. Setup Context
	wsID := "ws_single"
	docID := "doc_single"
	b := &base.Context{
		Repo: store,
		Job: &base.JobContext{
			WorkspaceID: wsID,
			DocumentID:  docID,
		},
	}

	t.Run("Process single PDF and verify directory structure", func(t *testing.T) {
		source := domain.SourceFile{
			Filename:    "report.pdf",
			URI:         "https://example.com/report.pdf",
			MimeType:    "application/pdf",
			Content:     []byte("%PDF-1.4 Fake PDF Content"),
			WorkspaceID: wsID,
			DocumentID:  docID,
		}

		// Save to FUSE
		dir := sourcefiles.FUSE.ResolvePath(wsID, docID)
		os.MkdirAll(dir, 0755)
		destPath := filepath.Join(dir, source.Filename)
		os.WriteFile(destPath, source.Content, 0644)
		
		fileRecord, _ := store.CreateDocumentFile(context.Background(), docID, source.Filename, source.MimeType, int64(len(source.Content)))
		res, err := processSingleTextFile(context.Background(), b, source, fileRecord)
		require.NoError(t, err)

		// Verify markers
		assert.Contains(t, res.RawText, "--- File: report.pdf (ID: file-report.pdf) ---")
		
		// Verify FUSE file exists
		if _, err := os.Stat(destPath); err != nil {
			t.Errorf("single file not saved to FUSE: %v", err)
		}
	})
}

func TestIsLikelyText(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		mimeType string
		want     bool
	}{
		{"Empty content", []byte(""), "application/octet-stream", true},
		{"Simple text", []byte("Hello World"), "text/plain", true},
		{"Go code", []byte("package main\nfunc main() {}"), "application/octet-stream", true},
		{"Binary image", []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}, "image/jpeg", false},
		{"JSON content", []byte(`{"key": "value"}`), "application/json", true},
		{"Text with NULL byte", []byte("Text\x00Binary"), "text/plain", false},
		{"Invalid UTF-8", []byte{0xff, 0xfe, 0xfd}, "application/octet-stream", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLikelyText(tt.content, tt.mimeType); got != tt.want {
				t.Errorf("isLikelyText() = %v, want %v", got, tt.want)
			}
		})
	}
}
