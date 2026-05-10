package io

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"github.com/synthify/backend/packages/shared/repository/mock"
	"github.com/synthify/backend/packages/shared/storage"
)

func TestGrepTool_FileIDPopulation(t *testing.T) {
	// 1. Setup temporary FS mount
	tmpDir := t.TempDir()
	fs := storage.NewFileSystem(tmpDir)

	wsID := "ws_123"
	docID := "doc_456"
	docDir := filepath.Join(tmpDir, wsID, docID)
	err := os.MkdirAll(docDir, 0755)
	require.NoError(t, err)

	// Create a nested file
	relPath := "src/main.go"
	filePath := filepath.Join(docDir, relPath)
	err = os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filePath, []byte("package main\nfunc main() {\n\tfmt.Println(\"FOUND-ME\")\n}"), 0644)
	require.NoError(t, err)

	// 2. Setup Mock Repository with the file record
	store := mock.NewStore()
	mockFile, err := store.CreateDocumentFile(context.Background(), docID, relPath, "text/x-go", 100)
	require.NoError(t, err)

	b := &base.Context{
		Repo: store,
		FS:   fs,
		Job: &base.JobContext{
			WorkspaceID: wsID,
			DocumentID:  docID,
		},
	}

	t.Run("Verify FileID is populated from repo", func(t *testing.T) {
		args := GrepArgs{Pattern: "FOUND-ME"}
		result, err := grepSearch(context.Background(), b, args)
		require.NoError(t, err)

		require.Len(t, result.Matches, 1)
		assert.Equal(t, relPath, result.Matches[0].FilePath)
		assert.Equal(t, mockFile.FileID, result.Matches[0].FileID, "FileID should match the record in the repo")
	})

	t.Run("Verify FileID is preserved in cache", func(t *testing.T) {
		pattern := "FOUND-ME"
		args := GrepArgs{Pattern: pattern}

		// First call (populates cache with FileID)
		_, err := grepSearch(context.Background(), b, args)
		require.NoError(t, err)

		// Clear mock repo to ensure only cache can provide the result
		// (Actually, if we implement the cache correctly, it should store the FileID)
		
		// Second call (hits cache)
		result, err := grepSearch(context.Background(), b, args)
		require.NoError(t, err)

		require.Len(t, result.Matches, 1)
		assert.Equal(t, mockFile.FileID, result.Matches[0].FileID, "FileID should be restored from cache")
	})

	t.Run("Handle missing file records gracefully", func(t *testing.T) {
		// Create a file that IS on disk but NOT in the repo
		ghostPath := "ghost.txt"
		err := os.WriteFile(filepath.Join(docDir, ghostPath), []byte("BOO"), 0644)
		require.NoError(t, err)

		args := GrepArgs{Pattern: "BOO"}
		result, err := grepSearch(context.Background(), b, args)
		require.NoError(t, err)

		require.Len(t, result.Matches, 1)
		assert.Equal(t, ghostPath, result.Matches[0].FilePath)
		assert.Empty(t, result.Matches[0].FileID, "FileID should be empty if not found in repo")
	})

	t.Run("Handle special characters in paths", func(t *testing.T) {
		specialPath := "docs/my report (v2).txt"
		fullSpecialPath := filepath.Join(docDir, specialPath)
		err := os.MkdirAll(filepath.Dir(fullSpecialPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullSpecialPath, []byte("SPECIAL-DATA"), 0644)
		require.NoError(t, err)

		mockFile, err := store.CreateDocumentFile(context.Background(), docID, specialPath, "text/plain", 100)
		require.NoError(t, err)

		args := GrepArgs{Pattern: "SPECIAL-DATA"}
		result, err := grepSearch(context.Background(), b, args)
		require.NoError(t, err)

		require.Len(t, result.Matches, 1)
		assert.Equal(t, specialPath, result.Matches[0].FilePath)
		assert.Equal(t, mockFile.FileID, result.Matches[0].FileID)
	})
}

func TestGrepTool(t *testing.T) {
	// 1. Setup temporary FS mount simulation
	tmpDir := t.TempDir()

	wsID := "test-ws"
	docID := "test-doc"
	wsPath := filepath.Join(tmpDir, wsID)
	err := os.MkdirAll(wsPath, 0755)
	require.NoError(t, err)

	content := `Line 1: Hello World
Line 2: This is a test file.
Line 3: Specialized keyword: SYNTHIFY-2026.
Line 4: Another line here.
Line 5: Case Insensitive Test.
Line 6: Goodbye.`
	
	err = os.WriteFile(filepath.Join(wsPath, docID), []byte(content), 0644)
	require.NoError(t, err)

	// 2. Initialize FS handler
	fs := storage.NewFileSystem(tmpDir)

	b := &base.Context{
		FS: fs,
		Job: &base.JobContext{
			WorkspaceID: wsID,
			DocumentID:  docID,
		},
	}

	ctx := context.Background()

	t.Run("Basic search", func(t *testing.T) {
		args := GrepArgs{Pattern: "Specialized"}
		result, err := grepSearch(ctx, b, args)
		require.NoError(t, err, "grepSearch failed")

		require.Len(t, result.Matches, 1, "Expected 1 match")
		assert.Equal(t, 3, result.Matches[0].LineNumber, "Expected match on line 3")
	})

	t.Run("Case insensitive search", func(t *testing.T) {
		args := GrepArgs{Pattern: "case insensitive", IgnoreCase: true}
		result, err := grepSearch(ctx, b, args)
		require.NoError(t, err, "grepSearch failed")

		assert.Len(t, result.Matches, 1, "Expected 1 match")
	})

	t.Run("Extended regex search", func(t *testing.T) {
		args := GrepArgs{Pattern: "SYNTHIFY-[0-9]{4}", ExtendedRegex: true}
		result, err := grepSearch(ctx, b, args)
		require.NoError(t, err, "grepSearch failed")

		assert.Len(t, result.Matches, 1, "Expected 1 match")
	})

	t.Run("Caching", func(t *testing.T) {
		pattern := "World"
		args := GrepArgs{Pattern: pattern}
		
		// First call (populates cache)
		_, err := grepSearch(ctx, b, args)
		require.NoError(t, err)

		// Verify cache file exists
		cacheDir := filepath.Join(tmpDir, ".cache", "v1", docID, "grep")
		files, err := os.ReadDir(cacheDir)
		require.NoError(t, err)
		require.NotEmpty(t, files, "Cache file was not created")

		// Second call (should hit cache)
		// We'll modify the source file to prove cache is used
		err = os.WriteFile(filepath.Join(wsPath, docID), []byte("Empty"), 0644)
		require.NoError(t, err)
		
		result, err := grepSearch(ctx, b, args)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Matches, "Should have returned cached result even though file changed")
	})
}
