package pipeline

type PipelineContext struct {
	JobID       string
	JobType     string
	DocumentID  string
	WorkspaceID string
	GraphID     string

	FileURI  string
	Filename string
	MimeType string

	SourceFiles []SourceFile

	RawText string

	Chunks  []Chunk
	Outline []string

	DocumentBrief *DocumentBrief
	SectionBriefs []SectionBrief

	Pass1Results map[int]Pass1ChunkResult

	SynthesizedNodes []SynthesizedNode
	SynthesizedEdges []SynthesizedEdge

	NodeIDMap map[string]string
}

type SourceFile struct {
	Filename string
	URI      string
	MimeType string
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

type Pass1ChunkResult struct {
	ChunkIndex int
	Nodes      []RawNode
}

type RawNode struct {
	LocalID        string
	Label          string
	Level          int
	EntityType     string
	Description    string
	SourceChunkIDs []string
}

type SynthesizedNode struct {
	LocalID        string
	Label          string
	Level          int
	EntityType     string
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
