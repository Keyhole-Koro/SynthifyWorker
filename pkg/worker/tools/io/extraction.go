package io

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
	"github.com/synthify/backend/apps/worker/pkg/worker/sourcefiles"
	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"github.com/synthify/backend/packages/shared/domain"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type ExtractionArgs struct {
	FileURI     string `json:"file_uri"`
	MimeType    string `json:"mime_type"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	DocumentID  string `json:"document_id,omitempty"`
}

type ExtractionResult struct {
	RawText string `json:"raw_text"`
}

func NewExtractionTool(b *base.Context) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "extract_text",
		Description: "Extracts raw text from a given document URI (PDF, TXT, ZIP, etc.) or transcribes media files (audio/video).",
	}, func(ctx tool.Context, args ExtractionArgs) (ExtractionResult, error) {
		wsID := args.WorkspaceID
		if wsID == "" && b != nil && b.Job != nil {
			wsID = b.Job.WorkspaceID
		}
		docID := args.DocumentID
		if docID == "" && b != nil && b.Job != nil {
			docID = b.Job.DocumentID
		}

		source := domain.SourceFile{
			URI:         args.FileURI,
			MimeType:    args.MimeType,
			WorkspaceID: wsID,
			DocumentID:  docID,
		}
		if err := sourcefiles.Fetch(ctx, &source); err != nil {
			return ExtractionResult{}, err
		}

		// Routing: ZIP vs Single File vs Media
		if isZip(source.MimeType, source.Filename) {
			return processZip(ctx, b, source)
		}

		if isMediaFile(source.MimeType) {
			return processMedia(ctx, b, source)
		}

		return processTextFile(ctx, b, source)
	})
}

func isZip(mimeType, filename string) bool {
	return mimeType == "application/zip" || 
		mimeType == "application/x-zip-compressed" || 
		strings.HasSuffix(strings.ToLower(filename), ".zip")
}

func isMediaFile(mimeType string) bool {
	return strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "video/")
}

func processTextFile(ctx context.Context, b *base.Context, source domain.SourceFile) (ExtractionResult, error) {
	// For single files, we should also ensure a document_files record exists
	if b != nil && b.Repo != nil {
		_, _ = b.Repo.CreateDocumentFile(ctx, source.DocumentID, source.Filename, source.MimeType, int64(len(source.Content)))
	}

	text := string(source.Content)
	text = strings.ReplaceAll(text, "\x00", "")
	return ExtractionResult{RawText: strings.TrimSpace(text)}, nil
}

func processMedia(ctx context.Context, b *base.Context, source domain.SourceFile) (ExtractionResult, error) {
	if b == nil || b.LLM == nil {
		return ExtractionResult{}, fmt.Errorf("LLM client not configured for transcription")
	}

	// Record the media file as a document file entry
	if b.Repo != nil {
		_, _ = b.Repo.CreateDocumentFile(ctx, source.DocumentID, source.Filename, source.MimeType, int64(len(source.Content)))
	}

	text, err := b.LLM.GenerateText(ctx, llm.TextRequest{
		SystemPrompt: "You are an expert transcription and video analysis assistant. Your task is to provide a high-quality, verbatim transcription of the provided media file. If it is a video, describe key visual transitions with timestamps as well.",
		UserPrompt:   "Please transcribe this media file. Use MM:SS format for timestamps.",
		SourceFiles:  []domain.SourceFile{source},
	})
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("transcription failed: %w", err)
	}
	return ExtractionResult{RawText: strings.TrimSpace(text)}, nil
}

func processZip(ctx context.Context, b *base.Context, source domain.SourceFile) (ExtractionResult, error) {
	if sourcefiles.FUSE == nil || sourcefiles.FUSE.MountPath == "" {
		return ExtractionResult{}, fmt.Errorf("FUSE mount required for ZIP extraction")
	}

	// Extraction base path: /mnt/gcs/{wsID}/{docID}/
	extractDir := sourcefiles.FUSE.ResolvePath(source.WorkspaceID, source.DocumentID)
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return ExtractionResult{}, fmt.Errorf("failed to create extraction dir: %w", err)
	}

	r, err := zip.NewReader(bytes.NewReader(source.Content), int64(len(source.Content)))
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("invalid zip file: %w", err)
	}

	var combinedText strings.Builder
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		relPath := f.Name
		destPath := filepath.Join(extractDir, relPath)

		// Create parent dirs
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			continue
		}

		// Extract file
		rc, err := f.Open()
		if err != nil {
			continue
		}
		
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			log.Printf("failed to write extracted file %s: %v", relPath, err)
			continue
		}

		// Detect MIME and Record to DB
		mimeType := http.DetectContentType(content)
		if b != nil && b.Repo != nil {
			_, _ = b.Repo.CreateDocumentFile(ctx, source.DocumentID, relPath, mimeType, int64(len(content)))
		}

		// Only append text files to the combined result for the LLM's initial view
		if isLikelyText(content, mimeType) {
			combinedText.WriteString(fmt.Sprintf("\n--- File: %s ---\n", relPath))
			combinedText.Write(content)
			combinedText.WriteString("\n")
		}
	}

	return ExtractionResult{RawText: strings.TrimSpace(combinedText.String())}, nil
}
func isLikelyText(content []byte, mimeType string) bool {
	if len(content) == 0 {
		return true
	}

	// 1. Binary heuristic: scan first 512 bytes for NULL bytes.
	// We do this first because even if mislabeled as text, a NULL byte strongly indicates binary.
	checkLen := 512
	if len(content) < checkLen {
		checkLen = len(content)
	}
	for i := 0; i < checkLen; i++ {
		if content[i] == 0 {
			return false // Likely binary
		}
	}

	// 2. Known text types are usually text
	if strings.HasPrefix(mimeType, "text/") ||
		strings.Contains(mimeType, "json") ||
		strings.Contains(mimeType, "markdown") {
		return true
	}

	// 3. Last resort: must be valid UTF-8
	return utf8.Valid(content)
}

