package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/synthify/backend/apps/worker/pkg/worker/sourcefiles"
	"github.com/synthify/backend/packages/shared/config"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/job/log"
	"github.com/synthify/backend/packages/shared/storage"
	"google.golang.org/genai"
)

type GeminiClient struct {
	client *genai.Client
	model  string
	fs     *storage.FileSystem
}

func NewGeminiClient(ctx context.Context, cfg config.LLM, fs *storage.FileSystem) (*GeminiClient, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("init gemini client: %w", err)
	}

	return &GeminiClient{
		client: client,
		model:  cfg.GeminiModel,
		fs:     fs,
	}, nil
}

func (c *GeminiClient) GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, Usage, error) {
	if c.client == nil {
		return nil, Usage{}, fmt.Errorf("gemini client not initialized")
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		},
		Temperature:      ptr(float32(0.2)),
		ResponseMIMEType: "application/json",
	}

	res, err := c.generate(ctx, req.SystemPrompt, req.UserPrompt, req.SourceFiles, config)
	if err != nil {
		return nil, Usage{}, fmt.Errorf("gemini api: %w", err)
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return nil, geminiUsage(c.model, res), fmt.Errorf("gemini api: empty response")
	}

	text := res.Candidates[0].Content.Parts[0].Text
	return json.RawMessage(RepairJSON(text)), geminiUsage(c.model, res), nil
}

func (c *GeminiClient) GenerateText(ctx context.Context, req TextRequest) (string, Usage, error) {
	if c.client == nil {
		return "", Usage{}, fmt.Errorf("gemini client not initialized")
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		},
		Temperature: ptr(float32(0.2)),
	}

	res, err := c.generate(ctx, req.SystemPrompt, req.UserPrompt, req.SourceFiles, config)
	if err != nil {
		return "", Usage{}, fmt.Errorf("gemini api: %w", err)
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return "", geminiUsage(c.model, res), fmt.Errorf("gemini api: empty response")
	}

	return res.Candidates[0].Content.Parts[0].Text, geminiUsage(c.model, res), nil
}

func (c *GeminiClient) generate(ctx context.Context, systemPrompt, userPrompt string, sourceFiles []domain.SourceFile, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	contents, cleanup, err := c.buildContents(ctx, userPrompt, sourceFiles)
	if err != nil {
		return nil, fmt.Errorf("build contents: %w", err)
	}
	defer cleanup()
	start := time.Now()
	res, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
	durationMs := time.Since(start).Milliseconds()
	log.Printf("gemini: model=%s duration=%dms err=%v", c.model, durationMs, err)
	if err == nil {
		joblog.FromContext(ctx).Log(ctx, joblog.Event{
			Level:   joblog.INFO,
			Event:   "llm.call.completed",
			Message: fmt.Sprintf("gemini call completed: model=%s duration=%dms", c.model, durationMs),
			Detail:  map[string]any{"model": c.model, "duration_ms": durationMs},
		})
	}
	return res, err
}

func (c *GeminiClient) buildContents(ctx context.Context, userPrompt string, sourceFiles []domain.SourceFile) ([]*genai.Content, func(), error) {
	parts := []*genai.Part{{Text: userPrompt}}
	var uploadedNames []string
	cleanup := func() {
		for _, name := range uploadedNames {
			c.deleteUploadedFile(ctx, name)
		}
	}

	for _, source := range sourceFiles {
		uploaded, err := c.uploadSourceFile(ctx, source)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		uploadedNames = append(uploadedNames, uploaded.Name)
		parts = append(parts, genai.NewPartFromFile(*uploaded))
	}

	return []*genai.Content{{Parts: parts}}, cleanup, nil
}

func (c *GeminiClient) uploadSourceFile(ctx context.Context, source domain.SourceFile) (*genai.File, error) {
	if err := sourcefiles.Fetch(ctx, c.fs, &source); err != nil {
		return nil, fmt.Errorf("fetch source file %s: %w", source.Filename, err)
	}

	mimeType := detectMIMEType(source)
	uploaded, err := c.client.Files.Upload(ctx, bytes.NewReader(source.Content), &genai.UploadFileConfig{
		MIMEType:    mimeType,
		DisplayName: detectDisplayName(source),
	})
	if err != nil {
		return nil, fmt.Errorf("upload source file: %w", err)
	}
	if err := c.waitForFileActive(ctx, uploaded.Name); err != nil {
		return nil, fmt.Errorf("wait for source file processing: %w", err)
	}
	return c.client.Files.Get(ctx, uploaded.Name, nil)
}

func (c *GeminiClient) waitForFileActive(ctx context.Context, name string) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		file, err := c.client.Files.Get(ctx, name, nil)
		if err != nil {
			return fmt.Errorf("get file status %s: %w", name, err)
		}
		switch file.State {
		case "", genai.FileStateActive:
			return nil
		case genai.FileStateFailed:
			return fmt.Errorf("file processing failed")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for uploaded file to become active")
		case <-ticker.C:
		}
	}
}

func (c *GeminiClient) deleteUploadedFile(ctx context.Context, name string) {
	if c.client == nil || strings.TrimSpace(name) == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, _ = c.client.Files.Delete(cleanupCtx, name, nil)
}

func detectMIMEType(source domain.SourceFile) string {
	if mimeType := strings.TrimSpace(source.MimeType); mimeType != "" {
		return mimeType
	}
	if extType := mime.TypeByExtension(path.Ext(source.Filename)); extType != "" {
		return extType
	}
	if len(source.Content) > 0 {
		return http.DetectContentType(source.Content)
	}
	return "application/octet-stream"
}

func detectDisplayName(source domain.SourceFile) string {
	if name := strings.TrimSpace(source.Filename); name != "" {
		return name
	}
	if base := path.Base(source.URI); base != "." && base != "/" && base != "" {
		return base
	}
	return "source-file"
}

func geminiUsage(model string, res *genai.GenerateContentResponse) Usage {
	if res == nil || res.UsageMetadata == nil {
		return Usage{Model: model}
	}
	meta := res.UsageMetadata
	return Usage{
		Model:        model,
		InputTokens:  int64(meta.PromptTokenCount),
		OutputTokens: int64(meta.CandidatesTokenCount),
	}
}

func ptr[T any](v T) *T {
	return &v
}
