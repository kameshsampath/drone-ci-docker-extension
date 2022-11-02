package main

import (
	"context"
	"flag"
	"net"
	"os"

	"github.com/harness/drone-ci-docker-extension/pkg/handler"
	"github.com/harness/drone-ci-docker-extension/pkg/utils"
	echo "github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

func main() {
	var log *logrus.Logger
	var err error
	var socketPath, v, dbFile string

	flag.StringVar(&socketPath, "socket", "/run/guest/volumes-service.sock", "Unix domain socket to listen on")
	flag.StringVar(&dbFile, "dbPath", utils.LookupEnvOrString("DRONE_CI_EXTENSION_DB_FILE", "/data/db"), "File to store the Drone Pipeline Info")
	flag.StringVar(&v, "level", utils.LookupEnvOrString("LOG_LEVEL", logrus.InfoLevel.String()), "The log level to use. Allowed values trace,debug,info,warn,fatal,panic.")
	flag.Parse()

	os.RemoveAll(socketPath)

	log = utils.LogSetup(os.Stdout, v)

	log.Infof("Starting listening on %s\n", socketPath)
	router := echo.New()
	router.HideBanner = true

	startURL := ""

	ln, err := listen(socketPath)
	if err != nil {
		log.Fatal(err)
	}
	router.Listener = ln

	//Init DB
	ctx := context.Background()
	//add it the OS ENV
	if _, ok := os.LookupEnv("DRONE_CI_EXTENSION_DB_FILE"); !ok {
		os.Setenv("DRONE_CI_EXTENSION_DB_FILE", dbFile)
	}
	h := handler.NewHandler(ctx, dbFile, log)

	//Routes
	router.GET("/stages", h.GetStages)
	router.GET("/stage/:id", h.GetStage)
	router.GET("/stage/:pipelineFile", h.GetStagesByPipelineFile)
	router.POST("/stages", h.SaveStages)
	router.PATCH("/stage/status", h.UpdateStageStatus)
	router.PATCH("/step/status", h.UpdateStepStatus)
	router.PATCH("/stage/status/reset", h.ResetStepStatuses)
	router.DELETE("/stages", h.DeleteAllStages)
	router.DELETE("/stages/:id", h.DeleteStage)
	router.DELETE("/pipeline/:pipelineFile", h.DeletePipeline)
	//TODO stream or remove??
	router.GET("/stage/:pipelineid/logs", h.StageLogs)

	log.Fatal(router.Start(startURL))
}

func listen(path string) (net.Listener, error) {
	return net.Listen("unix", path)
}
