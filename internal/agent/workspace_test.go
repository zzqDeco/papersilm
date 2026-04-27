package agent

import "testing"

func TestExtractPromptPaperSourcesDedupesURLAndArxivID(t *testing.T) {
	t.Parallel()

	sources := extractPromptPaperSources("总结这篇论文 https://arxiv.org/abs/1706.03762 的核心贡献")
	if len(sources) != 1 {
		t.Fatalf("expected one deduped source, got %+v", sources)
	}
	if sources[0] != "https://arxiv.org/abs/1706.03762" {
		t.Fatalf("unexpected deduped source: %+v", sources)
	}
}
