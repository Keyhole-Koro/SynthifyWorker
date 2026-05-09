package io

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/apps/worker/pkg/worker/sourcefiles"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"github.com/synthify/backend/packages/shared/storage"
)

func TestGrepTool(t *testing.T) {
	// 1. Setup temporary FUSE mount simulation
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

	// 2. Initialize FUSE handler
	sourcefiles.FUSE = storage.NewFUSEHandler(tmpDir)
	defer func() { sourcefiles.FUSE = nil }()

	b := &base.Context{
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
