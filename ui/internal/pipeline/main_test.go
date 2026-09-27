package pipeline

import (
	"testing"

	"github.com/odevine/mimic/ui/internal/pipeline/pipelinetest"
)

func TestMain(m *testing.M) { pipelinetest.Run(m, &WritePlaceholders) }
