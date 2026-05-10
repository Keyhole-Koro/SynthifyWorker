package io

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/synthify/backend/apps/worker/pkg/worker/tools/base"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type GrepArgs struct {
	Pattern       string `json:"pattern"`
	ContextLines  int    `json:"context_lines,omitempty"`
	IgnoreCase    bool   `json:"ignore_case,omitempty"`
	ExtendedRegex bool   `json:"extended_regex,omitempty"`
	DocumentID    string `json:"document_id,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
}

type GrepMatch struct {
	FileID     string   `json:"file_id"`
	FilePath   string   `json:"file_path"`
	LineNumber int      `json:"line_number"`
	Line       string   `json:"line"`
	Before     []string `json:"before,omitempty"`
	After      []string `json:"after,omitempty"`
}

type GrepResult struct {
	Matches []GrepMatch `json:"matches"`
}

func NewGrepTool(b *base.Context) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "grep_search",
		Description: "Searches for a pattern in the document using grep. Returns matching lines and their context.",
	}, func(ctx tool.Context, args GrepArgs) (GrepResult, error) {
		return grepSearch(ctx, b, args)
	})
}

func grepSearch(ctx context.Context, b *base.Context, args GrepArgs) (GrepResult, error) {
	docID := args.DocumentID
	if docID == "" && b != nil && b.Job != nil {
		docID = b.Job.DocumentID
	}
	wsID := args.WorkspaceID
	if wsID == "" && b != nil && b.Job != nil {
		wsID = b.Job.WorkspaceID
	}

	if docID == "" || wsID == "" {
		return GrepResult{}, fmt.Errorf("document_id and workspace_id are required")
	}

	contextLines := args.ContextLines
	if contextLines <= 0 {
		contextLines = 5
	}

	// 1. Check Cache
	cacheKeyParts := fmt.Sprintf("%s|%d|%v|%v", args.Pattern, contextLines, args.IgnoreCase, args.ExtendedRegex)
	cacheKey := fmt.Sprintf("%x", sha256.Sum256([]byte(cacheKeyParts)))
	var cached GrepResult
	if b.FS != nil {
		found, err := b.FS.ReadCache(docID, "grep", cacheKey, &cached)
		if err == nil && found {
			return cached, nil
		}
	}

	// 2. Execute Grep
	if b.FS == nil || b.FS.MountPath == "" {
		return GrepResult{}, fmt.Errorf("FUSE mount is not available for grep_search")
	}

	targetPath := b.FS.DocPath(wsID, docID)
	// We use -r (recursive) to support zip-extracted directories, and -n (line number)
	// -H (always print filename) for consistent parsing.
	// -B and -A for context.
	cmdArgs := []string{"-r", "-n", "-H"}
	if args.IgnoreCase {
		cmdArgs = append(cmdArgs, "-i")
	}
	if args.ExtendedRegex {
		cmdArgs = append(cmdArgs, "-E")
	}
	cmdArgs = append(cmdArgs,
		"-B", strconv.Itoa(contextLines),
		"-A", strconv.Itoa(contextLines),
		args.Pattern,
		targetPath,
	)

	out, err := exec.CommandContext(ctx, "grep", cmdArgs...).CombinedOutput()
	// grep returns exit code 1 if no matches found, which is not an "error" for us
	if err != nil && !strings.Contains(err.Error(), "exit status 1") {
		return GrepResult{}, fmt.Errorf("grep command failed: %w (output: %s)", err, string(out))
	}

	// 3. Parse Output
	result := parseGrepOutput(string(out), targetPath)

	// Resolve FileID for each match from the repository
	if b != nil && b.Repo != nil {
		for i := range result.Matches {
			file, err := b.Repo.GetDocumentFileByPath(ctx, docID, result.Matches[i].FilePath)
			if err == nil && file != nil {
				result.Matches[i].FileID = file.FileID
			}
		}
	}

	// 4. Save Cache
	if b.FS != nil {
		_ = b.FS.WriteCache(docID, "grep", cacheKey, result)
	}

	return result, nil
}

func parseGrepOutput(output, targetPath string) GrepResult {
	if output == "" {
		return GrepResult{Matches: []GrepMatch{}}
	}

	var matches []GrepMatch
	lines := strings.Split(output, "\n")

	var currentMatch *GrepMatch

	for _, line := range lines {
		if line == "" || line == "--" {
			continue
		}

		// Robust parsing: grep output format is <path><sep><line><sep><content>
		// where <sep> is ':' for matches and '-' for context.

		var separator string
		isContext := false

		// Find the separator after the file path.
		// If targetPath is a file, the line starts with targetPath.
		// If targetPath is a directory, the line starts with targetPath + "/".

		idx := -1
		if strings.HasPrefix(line, targetPath+":") {
			idx = len(targetPath)
			separator = ":"
			isContext = false
		} else if strings.HasPrefix(line, targetPath+"-") {
			idx = len(targetPath)
			separator = "-"
			isContext = true
		} else if strings.HasPrefix(line, targetPath+"/") {
			// Find the first ':' or '-' after targetPath + "/"
			cIdx := strings.Index(line[len(targetPath)+1:], ":")
			hIdx := strings.Index(line[len(targetPath)+1:], "-")

			if cIdx != -1 && (hIdx == -1 || cIdx < hIdx) {
				idx = len(targetPath) + 1 + cIdx
				separator = ":"
				isContext = false
			} else if hIdx != -1 {
				idx = len(targetPath) + 1 + hIdx
				separator = "-"
				isContext = true
			}
		}

		if idx == -1 {
			continue
		}

		filePath := line[:idx]
		remainder := line[idx+1:]

		// Remainder should be <line><sep><content>
		parts := strings.SplitN(remainder, separator, 2)
		if len(parts) < 2 {
			continue
		}

		lineNum, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		content := parts[1]

		// Clean up relative path if it's inside targetPath
		displayPath := strings.TrimPrefix(filePath, targetPath)
		displayPath = strings.TrimPrefix(displayPath, "/")

		if !isContext {
			// This is a primary match line
			match := GrepMatch{
				FilePath:   displayPath,
				LineNumber: lineNum,
				Line:       content,
			}
			matches = append(matches, match)
			currentMatch = &matches[len(matches)-1]
		} else if currentMatch != nil {
			// This is a context line for the last seen match
			if lineNum < currentMatch.LineNumber {
				currentMatch.Before = append(currentMatch.Before, content)
			} else {
				currentMatch.After = append(currentMatch.After, content)
			}
		}
	}

	return GrepResult{Matches: matches}
}
