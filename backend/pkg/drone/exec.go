package drone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	r "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/drone-runners/drone-runner-docker/engine"
	"github.com/drone-runners/drone-runner-docker/engine/compiler"
	"github.com/drone-runners/drone-runner-docker/engine/linter"
	"github.com/drone-runners/drone-runner-docker/engine/resource"
	"github.com/harness/drone-ci-docker-extension/pkg/utils"

	"github.com/drone/drone-go/drone"
	"github.com/drone/envsubst"
	"github.com/drone/runner-go/environ"
	"github.com/drone/runner-go/environ/provider"
	"github.com/drone/runner-go/labels"
	"github.com/drone/runner-go/logger"
	"github.com/drone/runner-go/manifest"
	"github.com/drone/runner-go/pipeline"
	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/drone/runner-go/registry"
	"github.com/drone/runner-go/secret"
	"github.com/drone/signal"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

const (
	darwinExtensionSocketPath = "Library/Containers/com.docker.docker/Data/ext-sockets/drone_drone-ci-docker-extension/extension-drone-ci.sock"
	//LabelPipelineFile is to identify the pipeline file
	LabelPipelineFile = "io.drone.desktop.pipeline.file"
	//LabelIncludes is to hold list of included steps as comma separated string
	LabelIncludes = "io.drone.desktop.pipeline.includes"
	//LabelExcludes is to hold list of excluded steps as comma separated string
	LabelExcludes = "io.drone.desktop.pipeline.excludes"
	//LabelStageName is to identify the stage name
	LabelStageName = "io.drone.stage.name"
	//LabelStepName is to identify the step name
	LabelStepName = "io.drone.step.name"
	//LabelStepNumber is to identify the step number
	LabelStepNumber = "io.drone.step.number"
	//LabelService to identify if the step is a "Service"
	LabelService = "io.drone.desktop.pipeline.service"
)

var (
	socketPath     = "backend-service-socket.sock"
	nocontext      = context.Background()
	log            = utils.LogSetup(os.Stdout, "info")
	droneCIHome    string
	droneCILogsDir string
)

// Command exports the exec command.
var Command = &cli.Command{
	Name:      "exec",
	Usage:     "execute a local build",
	ArgsUsage: "[path/to/.drone.yml]",
	Before: func(ctx *cli.Context) error {
		if err := createSocketLink(ctx); err != nil {
			log.Fatalln(err)
		}
		return nil
	},
	Action: func(ctx *cli.Context) error {
		if err := exec(ctx); err != nil {
			log.Fatalln(err)
		}
		return nil
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "pipeline",
			Usage: "Name of the pipeline to execute",
		},
		&cli.StringSliceFlag{
			Name:  "include",
			Usage: "Name of steps to include",
		},
		&cli.StringSliceFlag{
			Name:  "exclude",
			Usage: "Name of steps to exclude",
		},
		&cli.StringFlag{
			Name:  "resume-at",
			Usage: "Name of start to resume at",
		},
		&cli.BoolFlag{
			Name:  "trusted",
			Usage: "build is trusted",
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "build timeout",
			Value: time.Hour,
		},
		&cli.StringSliceFlag{
			Name:  "volume",
			Usage: "build volumes",
		},
		&cli.StringSliceFlag{
			Name:  "network",
			Usage: "external networks",
		},
		&cli.StringFlag{
			Name:  "registry",
			Usage: "registry file",
		},
		&cli.StringFlag{
			Name:    "secret-file",
			Aliases: []string{"secrets"},
			Usage:   "secret file, define values that can be used with from_secret",
		},
		&cli.StringFlag{
			Name:  "env-file",
			Usage: "env file",
		},
		&cli.StringSliceFlag{
			Name:  "privileged",
			Usage: "privileged plugins",
			Value: cli.NewStringSlice(
				"plugins/docker",
				"plugins/acr",
				"plugins/ecr",
				"plugins/gcr",
				"plugins/heroku",
			),
		},
	},
}

func exec(cliContext *cli.Context) error {
	// lets do our mapping from CLI flags to an execCommand struct
	commy := toExecCommand(cliContext)
	rawsource, err := ioutil.ReadFile(commy.Source)
	if err != nil {
		return err
	}
	envs := environ.Combine(
		getEnv(cliContext),
		environ.System(commy.System),
		environ.Repo(commy.Repo),
		environ.Build(commy.Build),
		environ.Stage(commy.Stage),
		environ.Link(commy.Repo, commy.Build, commy.System),
		commy.Build.Params,
	)

	// string substitution function ensures that string
	// replacement variables are escaped and quoted if they
	// contain newlines.
	subf := func(k string) string {
		v := envs[k]
		if strings.Contains(v, "\n") {
			v = fmt.Sprintf("%q", v)
		}
		return v
	}

	// evaluates string replacement expressions and returns an
	// update configuration.
	config, err := envsubst.Eval(string(rawsource), subf)
	if err != nil {
		return err
	}

	// parse and lint the configuration.
	manifest, err := manifest.ParseString(config)
	if err != nil {
		return err
	}

	// a configuration can contain multiple pipelines.
	// get a specific pipeline resource for execution.
	if commy.Stage.Name == "" {
		log.Infoln("No stage specified, assuming 'default'")
		commy.Stage.Name = "default"
	}

	res, err := resource.Lookup(commy.Stage.Name, manifest)
	if err != nil {
		return fmt.Errorf("stage '%s' not found in build file : %w", commy.Stage.Name, err)
	}

	// lint the pipeline and return an error if any
	// linting rules are broken
	lint := linter.New()
	err = lint.Lint(res, commy.Repo)
	if err != nil {
		return err
	}

	// compile the pipeline to an intermediate representation.
	comp := &compiler.Compiler{
		Environ:    provider.Static(commy.Environ),
		Labels:     commy.Labels,
		Resources:  commy.Resources,
		Tmate:      commy.Tmate,
		Privileged: append(commy.Privileged, compiler.Privileged...),
		Networks:   commy.Networks,
		Volumes:    commy.Volumes,
		Secret:     secret.StaticVars(commy.Secrets),
		Registry: registry.Combine(
			registry.File(commy.Config),
		),
	}

	// when running a build locally cloning is always
	// disabled in favor of mounting the source code
	// from the current working directory.
	if !commy.Clone {
		pwd, _ := os.Getwd()
		comp.Mount = pwd
		//Add the new labels that helps looking up the step containers
		//by names
		if comp.Labels == nil {
			comp.Labels = make(map[string]string)
		}
		comp.Labels[LabelPipelineFile] = path.Join(pwd, commy.Source)
	}

	args := runtime.CompilerArgs{
		Pipeline: res,
		Manifest: manifest,
		Build:    commy.Build,
		Netrc:    commy.Netrc,
		Repo:     commy.Repo,
		Stage:    commy.Stage,
		System:   commy.System,
		Secret:   secret.StaticVars(commy.Secrets),
	}
	spec := comp.Compile(nocontext, args).(*engine.Spec)

	//Handle to parsed Pipeline
	p := res.(*resource.Pipeline)

	//As the Compiler does not add labels for Steps adding few here
	for i, step := range spec.Steps {
		extraLabels := map[string]string{}

		extraLabels[LabelStageName] = strings.TrimSpace(p.Name)
		extraLabels[LabelStepName] = strings.TrimSpace(step.Name)
		extraLabels[LabelStepNumber] = strconv.Itoa(i)

		//Know the includes while running the pipeline from the extension
		//TODO improve
		if len(commy.Include) > 0 {
			extraLabels[LabelIncludes] = strings.Join(commy.Include, ",")
		}

		//Know the excludes while running the pipeline from the extension
		if len(commy.Exclude) > 0 {
			extraLabels[LabelExcludes] = strings.Join(commy.Exclude, ",")
		}
		//Label the services from steps
		for _, svc := range p.Services {
			if b := step.Name == svc.Name; b {
				log.Tracef("%s Service == Step %s", svc.Name, step.Name)
				extraLabels[LabelService] = strconv.FormatBool(b)
				break
			}
		}
		step.Labels = labels.Combine(step.Labels, extraLabels)

		log.Tracef("Step %s, Labels: %#v", step.Name, step.Labels)
	}

	// include only steps that are in the include list,
	// if the list in non-empty.
	if len(commy.Include) > 0 {
	I:
		for _, step := range spec.Steps {
			if step.Name == "clone" {
				continue
			}
			for _, name := range commy.Include {
				if step.Name == name {
					continue I
				}
			}
			step.RunPolicy = runtime.RunNever
		}
	}
	// exclude steps that are in the exclude list, if the list in non-empty.
	if len(commy.Exclude) > 0 {
	E:
		for _, step := range spec.Steps {
			if step.Name == "clone" {
				continue
			}
			for _, name := range commy.Exclude {
				if step.Name == name {
					step.RunPolicy = runtime.RunNever
					continue E
				}
			}
		}
	}
	// resume at a specific step
	if cliContext.String("resume-at") != "" {
		for _, step := range spec.Steps {
			if step.Name == cliContext.String("resume-at") {
				break
			}
			if step.Name == "clone" {
				continue
			}
			for _, name := range commy.Exclude {
				if step.Name == name {
					step.RunPolicy = runtime.RunNever
					continue
				}
			}
		}
	}
	// create a step object for each pipeline step.
	for _, step := range spec.Steps {
		if step.RunPolicy == runtime.RunNever {
			continue
		}

		commy.Stage.Steps = append(commy.Stage.Steps, &drone.Step{
			StageID:   commy.Stage.ID,
			Number:    len(commy.Stage.Steps) + 1,
			Name:      step.Name,
			Status:    drone.StatusPending,
			ErrIgnore: step.ErrPolicy == runtime.ErrIgnore,
		})
	}

	// configures the pipeline timeout.
	timeout := time.Duration(commy.Repo.Timeout) * time.Minute
	ctx, cancel := context.WithTimeout(nocontext, timeout)
	defer cancel()

	// listen for operating system signals and cancel execution when received.
	ctx = signal.WithContextFunc(ctx, func() {
		println("received signal, terminating process")
		cancel()
	})

	state := &pipeline.State{
		Build:  commy.Build,
		Stage:  commy.Stage,
		Repo:   commy.Repo,
		System: commy.System,
	}

	// enable debug logging
	if commy.Debug {
		log.SetLevel(logrus.DebugLevel)
	}
	if commy.Trace {
		log.SetLevel(logrus.TraceLevel)
	}
	logger.Default = logger.Logrus(
		logrus.NewEntry(
			log,
		),
	)

	engine, err := engine.NewEnv(engine.Opts{})
	if err != nil {
		return err
	}

	pipelineID := utils.Md5OfString(commy.Source)

	// JSON Log Streamer
	streamer, err := newStreamer(pipelineID)
	if err != nil {
		dump(pipelineID, map[string]interface{}{
			"error": err.Error(),
			"state": state,
		})
		return err
	}

	// Update Status in DB
	reporter, err := newDBReporter(ctx, commy.Source, commy.Stage.Name)
	if err != nil {
		dump(pipelineID, map[string]interface{}{
			"error": err.Error(),
			"state": state,
		})
		return err
	}

	err = runtime.NewExecer(
		reporter,
		streamer,
		pipeline.NopUploader(),
		engine,
		commy.Procs,
	).Exec(ctx, spec, state)

	defer streamer.writer.Close()

	if err != nil {
		dump(pipelineID, map[string]interface{}{
			"error": err.Error(),
			"state": state,
		})
		return err
	}

	switch state.Stage.Status {
	case drone.StatusError, drone.StatusFailing, drone.StatusKilled:
		os.Exit(1)
	}
	return nil
}

func dump(pipelineID string, m map[string]interface{}) {
	errLog := path.Join(droneCILogsDir, fmt.Sprintf("%s-err.json", pipelineID))
	f, err := os.OpenFile(errLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	var enc *json.Encoder
	if err != nil {
		log.Errorf("Error Opening file %s, %s,using stdout", errLog, err)
		enc = json.NewEncoder(os.Stdout)
	} else {
		enc = json.NewEncoder(f)
	}
	enc.SetIndent("", "  ")
	_ = enc.Encode(m)
	defer f.Close()
}

func mkdirs() error {
	droneCIHome = path.Join(os.Getenv("HOME"), ".droneci")
	_, err := os.Stat(droneCIHome)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(droneCIHome, 0700)
		if err != nil {
			err = fmt.Errorf("unable to create %s,%w", droneCIHome, err)
			log.Error(err)
			return err
		}
	}
	droneCILogsDir = path.Join(droneCIHome, "logs")
	return os.MkdirAll(droneCILogsDir, 0700)
}

func createSocketLink(_ *cli.Context) error {
	if err := mkdirs(); err != nil {
		return err
	}
	switch r.GOOS {
	case "darwin":
		sp := path.Join(os.Getenv("HOME"), darwinExtensionSocketPath)
		_, err := os.Stat(sp)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			err := fmt.Errorf("extension socket %s not available,%w", sp, err)
			log.Error(err)
			return err
		}

		socketPath = path.Join(droneCIHome, socketPath)
		//TODO:check if symlink is valid, if not remove and recreate
		//os.Remove(slink)
		_, err = os.Stat(socketPath)
		if err != nil && errors.Is(err, os.ErrNotExist) {
			if err := os.Symlink(sp, socketPath); err != nil {
				err := fmt.Errorf("error creating socket symbolic link %s,%w", socketPath, err)
				log.Error(err)
				return err
			}
			log.Infof("Symlink %s created successfully", socketPath)
		}
		return nil
	//TODO
	case "windows":
		return nil
	//TODO
	case "linux":
		return nil
	}
	return nil
}
