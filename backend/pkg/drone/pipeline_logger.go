package drone

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/bfontaine/jsons"
	"github.com/drone/runner-go/pipeline"
	"github.com/harness/drone-ci-docker-extension/pkg/utils"
)

type JSONFileStreamer struct {
	seq    *sequence
	col    *sequence
	writer *jsons.FileWriter
}

var _ pipeline.Streamer = (*JSONFileStreamer)(nil)

func New(pipelineID string) (*JSONFileStreamer, error) {
	logFile := path.Join(utils.LookupEnvOrString("DRONE_CI_EXTENSION_LOGS_PATH", "/data/logs"), fmt.Sprintf("%s.json", pipelineID))
	fw := jsons.NewFileWriter(logFile)
	if err := fw.Open(); err != nil {
		return nil, err
	}
	return &JSONFileStreamer{
		seq:    new(sequence),
		col:    new(sequence),
		writer: fw,
	}, nil
}

// Stream implements pipeline.Streamer
func (j *JSONFileStreamer) Stream(_ context.Context, _ *pipeline.State, name string) io.WriteCloser {
	return &jsonlogger{
		writer: j.writer,
		seq:    j.seq,
		name:   name,
	}
}
