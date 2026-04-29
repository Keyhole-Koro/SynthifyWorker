package tools

import (
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"github.com/synthify/backend/worker/pkg/worker/sourcefiles"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type ExtractionArgs struct {
	FileURI  string `json:"file_uri"`
	MimeType string `json:"mime_type"`
}

type ExtractionResult struct {
	RawText string `json:"raw_text"`
}

func NewExtractionTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "extract_text",
		Description: "Extracts raw text from a given document URI (PDF, TXT, etc.).",
	}, func(ctx tool.Context, args ExtractionArgs) (ExtractionResult, error) {
		source := pipeline.SourceFile{URI: args.FileURI, MimeType: args.MimeType}
		if err := sourcefiles.Fetch(ctx, &source); err != nil {
			return ExtractionResult{}, err
		}
		text := string(source.Content)
		text = strings.ReplaceAll(text, "\x00", "")
		return ExtractionResult{RawText: strings.TrimSpace(text)}, nil
	})
}
