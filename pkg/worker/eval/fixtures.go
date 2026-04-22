package eval

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/synthify/backend/worker/pkg/worker/pipeline"
)

//go:embed fixtures/*.json
var fixtureFS embed.FS

type Fixture struct {
	ID          string             `json:"id"`
	Description string             `json:"description"`
	Stage       pipeline.StageName `json:"stage"`
	Input       FixtureInput       `json:"input"`
	Expected    FixtureExpected    `json:"expected"`
	LLMResponse json.RawMessage    `json:"llm_response"`
}

type FixtureInput struct {
	Filename      string                `json:"filename"`
	RawText       string                `json:"raw_text"`
	SourceFiles   []FixtureSourceFile   `json:"source_files"`
	Chunks        []FixtureChunk        `json:"chunks"`
	Outline       []string              `json:"outline"`
	DocumentBrief *FixtureDocumentBrief `json:"document_brief"`
	SectionBriefs []FixtureSectionBrief `json:"section_briefs"`
}

type FixtureSourceFile struct {
	Filename string `json:"filename"`
	URI      string `json:"uri"`
	MimeType string `json:"mime_type"`
	Content  string `json:"content"`
}

type FixtureChunk struct {
	ChunkIndex int    `json:"chunk_index"`
	Heading    string `json:"heading"`
	Text       string `json:"text"`
}

type FixtureDocumentBrief struct {
	Topic        string   `json:"topic"`
	Level01Hints []string `json:"level01_hints"`
	ClaimSummary string   `json:"claim_summary"`
	Entities     []string `json:"entities"`
	Outline      []string `json:"outline"`
}

type FixtureSectionBrief struct {
	Heading         string   `json:"heading"`
	Topic           string   `json:"topic"`
	NodeCandidates  []string `json:"node_candidates"`
	ConnectionHints string   `json:"connection_hints"`
}

type FixtureExpected struct {
	MinNodeCount          int                 `json:"min_node_count"`
	MinChunkCount         int                 `json:"min_chunk_count"`
	RequiredLabels        []string            `json:"required_labels"`
	RequiredOutline       []string            `json:"required_outline"`
	RequiredChunkHeadings []string            `json:"required_chunk_headings"`
	RequiredHierarchical  []FixtureEdgeExpect `json:"required_hierarchical_edges"`
	RequiredTypedEdges    []FixtureEdgeExpect `json:"required_typed_edges"`
}

type FixtureEdgeExpect struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	EdgeType string `json:"edge_type"`
}

func LoadFixtures() ([]Fixture, error) {
	entries, err := fs.ReadDir(fixtureFS, "fixtures")
	if err != nil {
		return nil, err
	}
	var fixtures []Fixture
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := "fixtures/" + entry.Name()
		data, err := fixtureFS.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var fixture Fixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		fixtures = append(fixtures, fixture)
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].ID < fixtures[j].ID })
	return fixtures, nil
}

func LoadFixturesForStage(stage pipeline.StageName) ([]Fixture, error) {
	fixtures, err := LoadFixtures()
	if err != nil {
		return nil, err
	}
	var filtered []Fixture
	for _, fixture := range fixtures {
		if fixture.Stage == stage {
			filtered = append(filtered, fixture)
		}
	}
	return filtered, nil
}

func (f Fixture) PipelineContext() *pipeline.PipelineContext {
	var documentBrief *pipeline.DocumentBrief
	if f.Input.DocumentBrief != nil {
		documentBrief = &pipeline.DocumentBrief{
			Topic:        f.Input.DocumentBrief.Topic,
			Level01Hints: append([]string(nil), f.Input.DocumentBrief.Level01Hints...),
			ClaimSummary: f.Input.DocumentBrief.ClaimSummary,
			Entities:     append([]string(nil), f.Input.DocumentBrief.Entities...),
			Outline:      append([]string(nil), f.Input.DocumentBrief.Outline...),
		}
	}
	sectionBriefs := make([]pipeline.SectionBrief, 0, len(f.Input.SectionBriefs))
	for _, section := range f.Input.SectionBriefs {
		sectionBriefs = append(sectionBriefs, pipeline.SectionBrief{
			Heading:         section.Heading,
			Topic:           section.Topic,
			NodeCandidates:  append([]string(nil), section.NodeCandidates...),
			ConnectionHints: section.ConnectionHints,
		})
	}
	sourceFiles := make([]pipeline.SourceFile, 0, len(f.Input.SourceFiles))
	for _, source := range f.Input.SourceFiles {
		sourceFiles = append(sourceFiles, pipeline.SourceFile{
			Filename: source.Filename,
			URI:      source.URI,
			MimeType: source.MimeType,
			Content:  []byte(source.Content),
		})
	}
	chunks := make([]pipeline.Chunk, 0, len(f.Input.Chunks))
	for _, chunk := range f.Input.Chunks {
		chunks = append(chunks, pipeline.Chunk{
			ChunkIndex: chunk.ChunkIndex,
			Heading:    chunk.Heading,
			Text:       chunk.Text,
		})
	}
	return &pipeline.PipelineContext{
		Filename:      f.Input.Filename,
		SourceFiles:   sourceFiles,
		RawText:       f.Input.RawText,
		Chunks:        chunks,
		Outline:       append([]string(nil), f.Input.Outline...),
		DocumentBrief: documentBrief,
		SectionBriefs: sectionBriefs,
	}
}
