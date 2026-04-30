package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/Keyhole-Koro/SynthifyShared/config"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/synthify/backend/worker/pkg/worker/sourcefiles"
	"google.golang.org/genai"
)

type GeminiClient struct {
	client *genai.Client
	model  string
}

func NewGeminiClient(cfg config.LLM) *GeminiClient {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		// NewClient during construction might be risky if API key is missing,
		// but let's follow the pattern.
		return &GeminiClient{model: cfg.GeminiModel}
	}

	return &GeminiClient{
		client: client,
		model:  cfg.GeminiModel,
	}
}

func (c *GeminiClient) GenerateStructured(ctx context.Context, req StructuredRequest) (json.RawMessage, error) {
	if c.client == nil {
		return nil, fmt.Errorf("gemini client not initialized")
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
		return nil, fmt.Errorf("gemini api: %w", err)
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini api: empty response")
	}

	text := res.Candidates[0].Content.Parts[0].Text
	return json.RawMessage(RepairJSON(text)), nil
}

func (c *GeminiClient) GenerateText(ctx context.Context, req TextRequest) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("gemini client not initialized")
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		},
		Temperature: ptr(float32(0.2)),
	}

	res, err := c.generate(ctx, req.SystemPrompt, req.UserPrompt, req.SourceFiles, config)
	if err != nil {
		return "", fmt.Errorf("gemini api: %w", err)
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini api: empty response")
	}

	return res.Candidates[0].Content.Parts[0].Text, nil
}

func (c *GeminiClient) generate(ctx context.Context, systemPrompt, userPrompt string, sourceFiles []domain.SourceFile, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	contents, cleanup, err := c.buildContents(ctx, userPrompt, sourceFiles)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return c.client.Models.GenerateContent(ctx, c.model, contents, config)
}

func (c *GeminiClient) buildContents(ctx context.Context, userPrompt string, sourceFiles []domain.SourceFile) ([]*genai.Content, func(), error) {
	parts := []*genai.Part{{Text: userPrompt}}
	var uploadedNames []string
	cleanup := func() {
		for _, name := range uploadedNames {
			c.deleteUploadedFile(name)
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
	if err := sourcefiles.Fetch(ctx, &source); err != nil {
		return nil, err
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
			return err
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

func (c *GeminiClient) deleteUploadedFile(name string) {
	if c.client == nil || strings.TrimSpace(name) == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

func ptr[T any](v T) *T {
	return &v
}
