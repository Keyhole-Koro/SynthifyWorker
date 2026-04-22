package pipeline

import "github.com/Keyhole-Koro/SynthifyShared/domain"

type PipelineContext struct {
	JobID       string
	JobType     string
	DocumentID  string
	WorkspaceID string
	GraphID     string
	Capability  *domain.JobCapability

	FileURI  string
	Filename string
	MimeType string

	SourceFiles []SourceFile

	RawText string

	Chunks  []Chunk
	Outline []string

	DocumentBrief *DocumentBrief
	SectionBriefs []SectionBrief

	SynthesizedNodes []SynthesizedNode
	SynthesizedEdges []SynthesizedEdge

	NodeIDMap map[string]string
}

type SourceFile struct {
	Filename string
	URI      string
	MimeType string
	Content  []byte
}

type Chunk struct {
	ChunkIndex int
	Heading    string
	Text       string
}

type DocumentBrief struct {
	Topic        string
	Level01Hints []string
	ClaimSummary string
	Entities     []string
	Outline      []string
}

type SectionBrief struct {
	Heading         string
	Topic           string
	NodeCandidates  []string
	ConnectionHints string
}

type SynthesizedNode struct {
	LocalID        string
	Label          string
	Level          int
	Description    string
	SummaryHTML    string
	ParentLocalID  string
	ChildLocalIDs  []string
	SourceChunkIDs []string
}

type SynthesizedEdge struct {
	SourceLocalID  string
	TargetLocalID  string
	EdgeType       string
	Description    string
	SourceChunkIDs []string
}
