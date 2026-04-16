package stages

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

type TextExtractionStage struct{}

func (s *TextExtractionStage) Name() pipeline.StageName { return pipeline.StageTextExtraction }

func (s *TextExtractionStage) Run(ctx context.Context, pctx *pipeline.PipelineContext) error {
	var parts []string
	for _, file := range pctx.SourceFiles {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.URI, nil)
		if err != nil {
			return err
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return err
		}
		if res.StatusCode >= 400 {
			return fmt.Errorf("failed to fetch source file: %s", res.Status)
		}
		text := string(body)
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
