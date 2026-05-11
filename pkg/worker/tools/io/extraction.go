package io

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
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
		Description: "Extracts raw text from a given document URI (PDF, TXT, ZIP, etc.) or transcribes media files (audio/video). Unified as a directory-based project.",
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
			Filename:    filepath.Base(args.FileURI), // Fallback filename
			MimeType:    args.MimeType,
			WorkspaceID: wsID,
			DocumentID:  docID,
		}
		if err := sourcefiles.Fetch(ctx, b.FS, &source); err != nil {
			return ExtractionResult{}, err
		}

		// Ensure document directory exists on FUSE
		if b.FS != nil {
			dir := b.FS.DocPath(wsID, docID)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return ExtractionResult{}, fmt.Errorf("failed to create document dir: %w", err)
			}
		}

		// Routing: ZIP vs Single File
		if isZip(source.MimeType, source.Filename) {
			return processZip(ctx, b, source)
		}

		// Single file processing: Save to FUSE first then handle by type
		destPath := ""
		if b.FS != nil {
			destPath = filepath.Join(b.FS.DocPath(wsID, docID), source.Filename)
			if err := os.WriteFile(destPath, source.Content, 0644); err != nil {
				b.Logger.Error(ctx, "extraction.save_single_file_failed", err, map[string]any{"filename": source.Filename})
			}
		}

		var fileRecord *domain.DocumentFile
		if b != nil && b.Repo != nil {
			var err error
			fileRecord, err = b.Repo.CreateDocumentFile(ctx, source.DocumentID, source.Filename, source.MimeType, int64(len(source.Content)))
			if err != nil {
				b.Logger.Error(ctx, "extraction.create_db_record_failed", err, map[string]any{"filename": source.Filename})
			}

		}

		if isMediaFile(source.MimeType) {
			return processMedia(ctx, b, source, fileRecord)
		}

		return processSingleTextFile(ctx, b, source, fileRecord)
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

func processSingleTextFile(ctx context.Context, b *base.Context, source domain.SourceFile, record *domain.DocumentFile) (ExtractionResult, error) {
	text := string(source.Content)
	text = strings.ReplaceAll(text, "\x00", "")

	fileID := ""
	if record != nil {
		fileID = record.FileID
	}

	resultText := strings.TrimSpace(text)
	if fileID != "" {
		resultText = fmt.Sprintf("--- File: %s (ID: %s) ---\n%s", source.Filename, fileID, resultText)
	}

	return ExtractionResult{RawText: resultText}, nil
}

func processMedia(ctx context.Context, b *base.Context, source domain.SourceFile, record *domain.DocumentFile) (ExtractionResult, error) {
	if b == nil || b.LLM == nil {
		return ExtractionResult{}, fmt.Errorf("%w: LLM client not configured for transcription", domain.ErrCritical)
	}

	text, err := b.LLM.GenerateText(ctx, llm.TextRequest{
		SystemPrompt: "You are an expert transcription and video analysis assistant. Your task is to provide a high-quality, verbatim transcription of the provided media file. If it is a video, describe key visual transitions with timestamps as well.",
		UserPrompt:   "Please transcribe this media file. Use MM:SS format for timestamps.",
		SourceFiles:  []domain.SourceFile{source},
	})
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("transcription failed: %w", err)
	}

	fileID := ""
	if record != nil {
		fileID = record.FileID
	}

	resultText := strings.TrimSpace(text)
	if fileID != "" {
		resultText = fmt.Sprintf("--- Media File: %s (ID: %s) ---\n%s", source.Filename, fileID, resultText)
	}

	return ExtractionResult{RawText: resultText}, nil
}

func processZip(ctx context.Context, b *base.Context, source domain.SourceFile) (ExtractionResult, error) {
	if b.FS == nil || b.FS.MountPath == "" {
		return ExtractionResult{}, fmt.Errorf("%w: FUSE mount required for ZIP extraction", domain.ErrCritical)
	}

	// Extraction base path: /mnt/gcs/{wsID}/{docID}/
	extractDir := b.FS.DocPath(source.WorkspaceID, source.DocumentID)
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return ExtractionResult{}, fmt.Errorf("failed to create extraction dir: %w", err)
	}

	r, err := zip.NewReader(bytes.NewReader(source.Content), int64(len(source.Content)))
	if err != nil {
		return ExtractionResult{}, fmt.Errorf("%w: invalid zip file format", domain.ErrJobError)
	}

	var combinedText strings.Builder
	extractedCount := 0

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		relPath := f.Name
		destPath := filepath.Join(extractDir, relPath)

		// Create parent dirs
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			b.Logger.Error(ctx, "extraction.create_parent_dir_failed", err, map[string]any{"path": relPath})
			continue
		}

		// Extract file
		rc, err := f.Open()
		if err != nil {
			b.Logger.Error(ctx, "extraction.open_zip_file_failed", err, map[string]any{"path": relPath})
			continue
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			b.Logger.Error(ctx, "extraction.read_zip_file_failed", err, map[string]any{"path": relPath})
			continue
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			b.Logger.Error(ctx, "extraction.write_extracted_file_failed", err, map[string]any{"path": relPath})
			continue
		}

		extractedCount++

		// Detect MIME and Record to DB
		mimeType := http.DetectContentType(content)
		var fileRecord *domain.DocumentFile
		if b != nil && b.Repo != nil {
			var err error
			fileRecord, err = b.Repo.CreateDocumentFile(ctx, source.DocumentID, relPath, mimeType, int64(len(content)))
			if err != nil {
				b.Logger.Error(ctx, "extraction.create_db_record_failed", err, map[string]any{"path": relPath})
			}
		}

		// Only append text files to the combined result for the LLM's initial view
		if isLikelyText(content, mimeType) {
			fileID := ""
			if fileRecord != nil {
				fileID = fileRecord.FileID
			}
			combinedText.WriteString(fmt.Sprintf("\n--- File: %s (ID: %s) ---\n", relPath, fileID))
			combinedText.Write(content)
			combinedText.WriteString("\n")
		}
	}

	if extractedCount == 0 {
		return ExtractionResult{}, fmt.Errorf("%w: zip file contained no valid files to extract", domain.ErrJobError)
	}

	return ExtractionResult{RawText: strings.TrimSpace(combinedText.String())}, nil
}

func isLikelyText(content []byte, mimeType string) bool {
	if len(content) == 0 {
		return true
	}

	// 1. Binary heuristic: scan first 512 bytes for NULL bytes.
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
