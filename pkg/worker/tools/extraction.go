package tools

import (
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
		return ExtractionResult{RawText: "Extracted content stub"}, nil
	})
}
