package overrides

import (
	"testing"

	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/pipeline/pipelinetest"
)

func TestMain(m *testing.M) { pipelinetest.Run(m, &pipeline.WritePlaceholders) }
