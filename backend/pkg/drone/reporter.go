package drone

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"

	"github.com/drone/drone-go/drone"
	"github.com/drone/runner-go/pipeline"
	"github.com/harness/drone-ci-docker-extension/pkg/handler"
	"github.com/labstack/gommon/log"
)

type dbReporter struct {
	pipelineFile string
	http         http.Client
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func nopReaderCloser(r io.Reader) io.ReadCloser {
	return nopCloser{r}
}

func NewDBReporter(ctx context.Context, socketPath, pipelineFile string) *dbReporter {
	dbReporter := &dbReporter{
		pipelineFile: pipelineFile,
	}
	dbReporter.http = http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
	return dbReporter
}

// ReportStage implements pipeline.Reporter
func (d *dbReporter) ReportStage(ctx context.Context, state *pipeline.State) error {
	name := "default"
	if state.Stage.Name != "" {
		name = state.Stage.Name
	}

	var status string
	if state.Stage.Status == "" {
		status = "success"
	}

	req := &handler.UpdateReq{
		StageName:    name,
		PipelineFile: d.pipelineFile,
		Status:       status,
	}

	rJson, err := json.Marshal(req)
	if err != nil {
		return err
	}

	res, err := d.doPost(ctx, rJson)

	if err != nil || res.StatusCode != 200 {
		return err
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err == nil {
		log.Infof("ReportStage::Response Code %d, Response: %s", res.StatusCode, buf.String())
	}

	return nil
}

// ReportStep implements pipeline.Reporter
func (d *dbReporter) ReportStep(ctx context.Context, state *pipeline.State, stepName string) error {
	name := "default"
	if state.Stage.Name != "" {
		name = state.Stage.Name
	}

	var c *drone.Step
	for _, step := range state.Stage.Steps {
		if step.Name == stepName {
			c = step
			break
		}
	}

	if c.Status == "" {
		c.Status = "success"
	}

	req := &handler.UpdateReq{
		StageName:    name,
		PipelineFile: d.pipelineFile,
		StepName:     c.Name,
		Status:       c.Status,
	}

	rJson, err := json.Marshal(req)
	if err != nil {
		return err
	}

	res, err := d.doPost(ctx, rJson)
	if err != nil || res.StatusCode != 200 {
		return err
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err == nil {
		log.Infof("ReportStep::Response Code %d, Response: %s", res.StatusCode, buf.String())
	}

	return nil
}

func (d *dbReporter) doPost(ctx context.Context, data []byte) (*http.Response, error) {
	r := nopReaderCloser(bytes.NewReader(data))
	hc := d.http
	req, err := http.NewRequestWithContext(ctx, "POST", "http://unix/", r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return hc.Do(req)
}

var _ pipeline.Reporter = (*dbReporter)(nil)
