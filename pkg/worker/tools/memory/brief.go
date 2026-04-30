package memory

import (
	"strings"
	"sync"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
)

type Brief struct {
	mu    sync.RWMutex
	brief *domain.DocumentBrief
}

func NewBrief() *Brief {
	return &Brief{}
}

func (b *Brief) Set(brief domain.DocumentBrief) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.brief = &brief
}

func (b *Brief) RenderForPrompt() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.brief == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Document Brief\n")
	sb.WriteString("- **Topic**: ")
	sb.WriteString(b.brief.Topic)
	sb.WriteString("\n- **Summary**: ")
	sb.WriteString(b.brief.ClaimSummary)
	sb.WriteString("\n")
	return sb.String()
}
