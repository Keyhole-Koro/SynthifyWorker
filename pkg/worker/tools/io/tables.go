package io

import (
	"encoding/json"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type TableArgs struct {
	ChunkID string `json:"chunk_id" jsonschema:"description=The chunk containing a complex table"`
	Text    string `json:"text,omitempty" jsonschema:"description=Raw chunk text when available"`
}

type TableResult struct {
	TableJSON string `json:"table_json" jsonschema:"description=Structured representation of the table"`
}

func NewTableTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "extract_table_data",
		Description: "Specially parses complex tables from document chunks into structured JSON format.",
	}, func(ctx tool.Context, args TableArgs) (TableResult, error) {
		rows := parseMarkdownTable(args.Text)
		payload, err := json.Marshal(map[string]any{
			"chunk_id": args.ChunkID,
			"rows":     rows,
		})
		if err != nil {
			return TableResult{}, err
		}
		return TableResult{TableJSON: string(payload)}, nil
	})
}

func parseMarkdownTable(text string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "|") || strings.Trim(line, "|-: ") == "" {
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		row := make([]string, 0, len(parts))
		for _, part := range parts {
			row = append(row, strings.TrimSpace(part))
		}
		rows = append(rows, row)
	}
	return rows
}
