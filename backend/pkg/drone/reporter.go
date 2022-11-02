package drone

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/drone/drone-go/drone"
	"github.com/drone/runner-go/pipeline"
)

const reqTemplate = `{"pipelineFile": "%s", "stageName": "%s","stepName": "%s",
"status": "%s"}`

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

func newDBReporter(_ context.Context, pipelineFile string) *dbReporter {
	dbReporter := &dbReporter{
		pipelineFile: pipelineFile,
	}
	dbReporter.http = http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				log.Infof("Using socket %s", socketPath)
				return net.Dial("unix", socketPath)
			},
		},
	}
	return dbReporter
}

// ReportStage implements pipeline.Reporter
func (d *dbReporter) ReportStage(ctx context.Context, state *pipeline.State) error {
	stageName := "default"
	if state.Stage.Name != "" {
		stageName = state.Stage.Name
	}

	var status string
	if state.Stage.Status == "" {
		status = "success"
	} else {
		status = state.Stage.Status
	}

	data := fmt.Sprintf(reqTemplate, d.pipelineFile, stageName, "", status)

	res, err := d.doPatch(ctx, "/stage/status", data)

	if err != nil {
		return err
	} else if res.StatusCode >= 400 {
		return fmt.Errorf("error patching '/stage/status'  %s", res.Status)
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err == nil {
		log.Infof("ReportStage::Response Code %d, Response: %s", res.StatusCode, buf.String())
	}

	return nil
}

// ReportStep implements pipeline.Reporter
func (d *dbReporter) ReportStep(ctx context.Context, state *pipeline.State, stepName string) error {
	stageName := "default"
	if state.Stage.Name != "" {
		stageName = state.Stage.Name
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

	data := fmt.Sprintf(reqTemplate, d.pipelineFile, stageName, stepName, c.Status)

	// Update the stage status to be running when first stage is started
	// and running
	if i := runningStepIndex(state.Stage, stepName); i == 0 && c.Status == drone.StatusRunning {
		res, err := d.doPatch(ctx, "/stage/status", data)
		if err != nil {
			return err
		} else if res.StatusCode >= 400 {
			return fmt.Errorf("error patching '/stage/status'  %s", res.Status)
		}
	}

	// Update the Step Status
	res, err := d.doPatch(ctx, "/step/status", data)
	if err != nil {
		return err
	} else if res.StatusCode >= 400 {
		return fmt.Errorf("error patching '/step/status'  %s", res.Status)
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(res.Body); err == nil {
		log.Infof("ReportStep::Response Code %d, Response: %s", res.StatusCode, buf.String())
	}

	return nil
}

func (d *dbReporter) doPatch(ctx context.Context, path string, data string) (*http.Response, error) {
	url := fmt.Sprintf("http://unix%s", path)
	log.Infof("Posting to URI :%s, Data:%s", url, data)
	r := nopReaderCloser(bytes.NewBufferString(data))
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := d.http.Do(req)
	if err != nil || res.StatusCode >= 400 {
		return nil, fmt.Errorf("error with request %s, %s", path, res.Status)
	}
	log.Infof("Successful with request %s and data %s", url, data)
	return res, nil
}

func runningStepIndex(stage *drone.Stage, stepName string) int {
	var stepIdx int
	for i, st := range stage.Steps {
		if st.Name == stepName {
			stepIdx = i
			break
		}
	}
	return stepIdx
}

var _ pipeline.Reporter = (*dbReporter)(nil)
