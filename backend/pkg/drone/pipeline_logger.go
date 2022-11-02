package drone

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/bfontaine/jsons"
	"github.com/drone/runner-go/pipeline"
)

type jSONFileStreamer struct {
	seq    *sequence
	col    *sequence
	writer *jsons.FileWriter
}

var _ pipeline.Streamer = (*jSONFileStreamer)(nil)

func newStreamer(pipelineID string) (*jSONFileStreamer, error) {
	logFile := path.Join(droneCILogsDir, fmt.Sprintf("%s.json", pipelineID))
	fw := jsons.NewFileWriter(logFile)
	if err := fw.Open(); err != nil {
		return nil, err
	}
	return &jSONFileStreamer{
		seq:    new(sequence),
		col:    new(sequence),
		writer: fw,
	}, nil
}

// Stream implements pipeline.Streamer
func (j *jSONFileStreamer) Stream(_ context.Context, _ *pipeline.State, name string) io.WriteCloser {
	return &jsonlogger{
		writer: j.writer,
		seq:    j.seq,
		name:   name,
	}
}
