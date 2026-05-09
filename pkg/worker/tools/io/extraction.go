package io

import (
	"fmt"
	"strings"

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
		Description: "Extracts raw text from a given document URI (PDF, TXT, etc.) or transcribes media files (audio/video).",
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

		// Handle multimedia transcription
		if isMediaFile(source.MimeType) && b != nil && b.LLM != nil {
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

		text := string(source.Content)
		text = strings.ReplaceAll(text, "\x00", "")
		return ExtractionResult{RawText: strings.TrimSpace(text)}, nil
	})
}

func isMediaFile(mimeType string) bool {
	return strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "video/")
}
