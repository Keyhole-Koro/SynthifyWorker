package stages

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
	"github.com/synthify/backend/worker/pkg/worker/sourcefiles"
)

type TextExtractionStage struct{}

func (s *TextExtractionStage) Name() pipeline.StageName { return pipeline.StageTextExtraction }

func (s *TextExtractionStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	var parts []string
	for i := range pctx.SourceFiles {
		if err := sourcefiles.Fetch(ctx, &pctx.SourceFiles[i]); err != nil {
			return err
		}
		file := pctx.SourceFiles[i]
		text := string(file.Content)
		if strings.Contains(file.MimeType, "csv") {
			rows, err := csv.NewReader(strings.NewReader(text)).ReadAll()
			if err == nil {
				var lines []string
				for _, row := range rows {
					lines = append(lines, strings.Join(row, " | "))
				}
				text = strings.Join(lines, "\n")
			}
		}
		parts = append(parts, fmt.Sprintf("## %s\n%s", file.Filename, strings.TrimSpace(text)))
	}
	pctx.RawText = strings.TrimSpace(strings.Join(parts, "\n\n"))
	if pctx.RawText == "" {
		return fmt.Errorf("text extraction produced empty text")
	}
	return nil
}
