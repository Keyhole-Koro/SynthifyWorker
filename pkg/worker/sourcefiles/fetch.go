package sourcefiles

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

func EnsureFetched(ctx context.Context, files []pipeline.SourceFile) ([]pipeline.SourceFile, error) {
	out := make([]pipeline.SourceFile, len(files))
	copy(out, files)
	for i := range out {
		if err := Fetch(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func Fetch(ctx context.Context, file *pipeline.SourceFile) error {
	if file == nil {
		return fmt.Errorf("source file is nil")
	}
	if len(file.Content) > 0 {
		return nil
	}
	if strings.TrimSpace(file.URI) == "" {
		return fmt.Errorf("source file URI is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.URI, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("failed to fetch source file: %s", res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	file.Content = body
	if strings.TrimSpace(file.MimeType) == "" {
		if mediaType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type")); err == nil && mediaType != "" {
			file.MimeType = mediaType
		}
	}
	return nil
}
