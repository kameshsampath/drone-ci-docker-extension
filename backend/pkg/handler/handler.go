package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/docker/docker/client"
	"github.com/drone/drone-go/drone"
	"github.com/harness/drone-ci-docker-extension/pkg/db"
	"github.com/harness/drone-ci-docker-extension/pkg/utils"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

// UpdateReq helps to capture the Stage or Status Update Requests
type UpdateReq struct {
	PipelineFile string `json:"pipelineFile"`
	StageName    string `json:"stageName"`
	StepName     string `json:"stepName,omitempty"`
	Status       string `json:"status"`
}

func NewHandler(ctx context.Context, dbFile string, log *logrus.Logger) *Handler {
	dbc := db.New(
		db.WithContext(ctx),
		db.WithLogger(log),
		db.WithDBFile(dbFile),
	)
	dbc.Init(true)

	return &Handler{
		DatabaseConfig: dbc,
	}
}

// GetStages selects all the available stages from the backend. The selected stages are sorted in ascending using column `pipeline_file`
func (h *Handler) GetStages(c echo.Context) error {
	log := h.DatabaseConfig.Log
	log.Info("Get Stages")
	stages := make(db.Stages, 0)
	db := h.DatabaseConfig.DB

	err := db.NewSelect().
		Model(&stages).
		Relation("Steps").
		Order("pipeline_file ASC").
		Scan(h.DatabaseConfig.Ctx)

	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, stages)
}

// GetStagesByPipelineFile selects selects stages associated with a PipelineFile
func (h *Handler) GetStagesByPipelineFile(c echo.Context) error {
	log := h.DatabaseConfig.Log
	var pipelineFile string
	if err := echo.PathParamsBinder(c).
		String("pipelineFile", &pipelineFile).
		BindError(); err != nil {
		return err
	}
	log.Infof("Get Stage by %s", pipelineFile)

	var stages db.Stages
	db := h.DatabaseConfig.DB

	err := db.NewSelect().
		Model(&stages).
		Relation("Steps").
		Where("pipeline_file = ?", pipelineFile).
		Scan(h.DatabaseConfig.Ctx)

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, stages)
}

// GetStage selects selects a stage by id from the backend.
func (h *Handler) GetStage(c echo.Context) error {
	log := h.DatabaseConfig.Log
	var stageID int
	if err := echo.PathParamsBinder(c).
		Int("id", &stageID).
		BindError(); err != nil {
		return err
	}
	log.Infof("Get Stage %d", stageID)

	stage := &db.Stage{
		ID: stageID,
	}
	db := h.DatabaseConfig.DB

	err := db.NewSelect().
		Model(stage).
		Relation("Steps").
		WherePK().
		Scan(h.DatabaseConfig.Ctx)

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, stage)
}

// DeleteStages deletes one or more stage ids from the backend
func (h *Handler) DeleteAllStages(c echo.Context) error {
	log := h.DatabaseConfig.Log
	var stages []*db.Stage
	if err := c.Bind(&stages); err != nil {
		return err
	}
	log.Infof("Delete all stages and steps")
	ctx := h.DatabaseConfig.Ctx
	dbConn := h.DatabaseConfig.DB
	err := dbConn.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := dbConn.NewTruncateTable().
			Model((*db.StageStep)(nil)).
			ContinueIdentity().
			Cascade().
			Exec(ctx)
		if err != nil {
			return err
		}
		_, err = dbConn.NewTruncateTable().
			Model((*db.Stage)(nil)).
			ContinueIdentity().
			Cascade().
			Exec(ctx)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}
	//Clean the logs directory
	os.RemoveAll("/data/logs")
	return c.NoContent(http.StatusNoContent)
}

// DeletePipeline delete all the stages and its steps of a defined PipelineFile
func (h *Handler) DeletePipeline(c echo.Context) error {
	log := h.DatabaseConfig.Log
	var pipelineFile string
	if err := echo.PathParamsBinder(c).
		String("pipelineFile", &pipelineFile).
		BindError(); err != nil {
		return err
	}
	log.Infof("Delete Pipeline %s", pipelineFile)

	var stages db.Stages
	db := h.DatabaseConfig.DB

	err := db.NewSelect().
		Model(&stages).
		Column("id").
		Where("pipeline_file = ?", pipelineFile).
		Scan(h.DatabaseConfig.Ctx)

	if err != nil {
		return err
	}

	err = h.delete(stages)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusNoContent, stages)
}

func (h *Handler) DeleteStage(c echo.Context) error {
	log := h.DatabaseConfig.Log
	var stageID int
	if err := echo.PathParamsBinder(c).
		Int("id", &stageID).
		BindError(); err != nil {
		return err
	}

	log.Infof("Delete Stage %d", stageID)

	if err := h.delete([]*db.Stage{{ID: stageID}}); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) delete(stages db.Stages) error {
	ctx := h.DatabaseConfig.Ctx
	dbConn := h.DatabaseConfig.DB
	return dbConn.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		for _, stage := range stages {
			_, err := dbConn.NewDelete().
				Model((*db.StageStep)(nil)).
				Where("stage_id = ?", stage.ID).
				Exec(ctx)
			if err != nil {
				return err
			}
			os.RemoveAll(fmt.Sprintf("/data/logs/%d", stage.ID))
		}

		_, err := dbConn.NewDelete().
			Model(&stages).
			WherePK().
			Exec(ctx)
		if err != nil {
			return err
		}
		return nil
	})
}

// SaveStages saves one or more stage ids to the backend
func (h *Handler) SaveStages(c echo.Context) error {
	log := h.DatabaseConfig.Log
	ctx := h.DatabaseConfig.Ctx
	var stages []*db.Stage
	if err := c.Bind(&stages); err != nil {
		return err
	}
	log.Infof("Saving stages %v", stages)

	dbConn := h.DatabaseConfig.DB
	if err := dbConn.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := dbConn.NewInsert().
			Model(&stages).
			On("CONFLICT(name,pipeline_file) DO UPDATE").
			Set("name = excluded.name").
			Set("pipeline_file = excluded.pipeline_file").
			Set("pipeline_path = excluded.pipeline_path").
			Set("status = excluded.status").
			Exec(ctx)
		if err != nil {
			return err
		}
		//Insert or update steps
		for _, stage := range stages {
			steps := stage.Steps
			for _, s := range steps {
				s.StageID = stage.ID
			}
			_, err = dbConn.NewInsert().
				Model(&steps).
				On("CONFLICT(name,stage_id) DO UPDATE").
				Set("name = excluded.name").
				Set("status = excluded.status").
				Set("image = excluded.image").
				Exec(ctx)
			if err != nil {
				return err
			}
		}
		return err
	}); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, stages)
}

// StageLogs retrieves logs associated with the stage. It is streaming operation
// that continuously reads from file system file
func (h *Handler) StageLogs(c echo.Context) error {
	log := h.DatabaseConfig.Log
	var stageID int
	if err := echo.PathParamsBinder(c).
		Int("id", &stageID).
		BindError(); err != nil {
		return err
	}
	log.Infof("Getting logs for Stage %d", stageID)

	return nil
}

// UpdateStageStatus is used to update the stage status. Stage status could be
// one of the following:
// 0  - None
// 1  - Success
// 2  - Running
// 3  - Error
// 4  - Stopped/Killed
func (h *Handler) UpdateStageStatus(c echo.Context) error {
	log := h.DatabaseConfig.Log
	ctx := h.DatabaseConfig.Ctx
	dbConn := h.DatabaseConfig.DB
	stageUReq := &UpdateReq{}
	if err := c.Bind(stageUReq); err != nil {
		return err
	}

	if err := dbConn.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		log.Infof("Updating Stage : %#v", stageUReq)
		status := h.fromDroneStatus(stageUReq.Status)
		log.Infof("Updating Stage %s with status %s", stageUReq.StageName, status)
		_, err := dbConn.NewUpdate().
			Model((*db.Stage)(nil)).
			Set("status = ?", status).
			Where("name = ? and pipeline_file = ? ", stageUReq.StageName, stageUReq.PipelineFile).
			Exec(ctx)
		return err
	}); err != nil {
		return err
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	utils.TriggerUIRefresh(ctx, cli, log)
	return c.NoContent(http.StatusNoContent)
}

// UpdateStepStatus is used to update the step status. Step status could be
// one of the following:
// 0  - None
// 1  - Success
// 2  - Running
// 3  - Error
// 4  - Stopped/Killed
func (h *Handler) UpdateStepStatus(c echo.Context) error {
	log := h.DatabaseConfig.Log
	ctx := h.DatabaseConfig.Ctx
	dbConn := h.DatabaseConfig.DB
	stepUReq := &UpdateReq{}
	if err := c.Bind(stepUReq); err != nil {
		return err
	}

	if err := dbConn.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		status := h.fromDroneStatus(stepUReq.Status)
		log.Infof("Updating Step %s of Stage %s with status %s", stepUReq.StepName, stepUReq.StageName, status)
		stageQ := dbConn.NewSelect().
			Model((*db.Stage)(nil)).
			Column("id").
			Where("name = ? AND pipeline_file = ? ", stepUReq.StageName, stepUReq.PipelineFile, 1)
		_, err := dbConn.NewUpdate().
			Model((*db.StageStep)(nil)).
			Set("status = ?", status).
			Where("stage_id IN (?) AND name = ? ", stageQ, stepUReq.StepName).
			Exec(ctx)
		return err
	}); err != nil {
		return err
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	utils.TriggerUIRefresh(ctx, cli, log)
	return c.NoContent(http.StatusNoContent)
}

// ResetStepStatuses resets the step statuses to db.None
func (h *Handler) ResetStepStatuses(c echo.Context) error {
	log := h.DatabaseConfig.Log
	ctx := h.DatabaseConfig.Ctx
	dbConn := h.DatabaseConfig.DB
	stepUReq := &UpdateReq{}
	if err := c.Bind(stepUReq); err != nil {
		return err
	}

	if err := dbConn.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		log.Infof("Resetting Step statuses of Stage %#v to be none", stepUReq)
		stageQ := dbConn.NewSelect().
			Model((*db.Stage)(nil)).
			Column("id").
			Where("name = ? AND pipeline_file = ? ", stepUReq.StageName, stepUReq.PipelineFile, 1)
		_, err := dbConn.NewUpdate().
			Model((*db.StageStep)(nil)).
			Set("status = ?", db.None).
			Where("stage_id IN (?)", stageQ).
			Exec(ctx)
		log.Infoln("Step statuses reset to none")
		return err
	}); err != nil {
		return err
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	utils.TriggerUIRefresh(ctx, cli, log)
	return c.NoContent(http.StatusNoContent)
}

// CheckIfStageExists checks if the Stage exists in the backend
func (h *Handler) CheckIfStageExists(c echo.Context) bool {
	log := h.DatabaseConfig.Log
	ctx := h.DatabaseConfig.Ctx
	dbConn := h.DatabaseConfig.DB
	var stageID int
	err := echo.PathParamsBinder(c).
		Int("id", &stageID).
		BindError()
	if err != nil {
		return false
	}
	exists, err := dbConn.
		NewSelect().
		Model(&db.Stage{ID: stageID}).
		WherePK().
		Exists(ctx)
	if err != nil {
		log.Errorf("Error while checking stage %d exists %#v", stageID, err)
		return false
	}

	return exists
}

// CheckIfStepExists checks if the Stage exists in the backend
func (h *Handler) CheckIfStepExists(c echo.Context) bool {
	log := h.DatabaseConfig.Log
	ctx := h.DatabaseConfig.Ctx
	dbConn := h.DatabaseConfig.DB
	var stepID int
	err := echo.PathParamsBinder(c).
		Int("id", &stepID).
		BindError()
	if err != nil {
		return false
	}

	exists, err := dbConn.
		NewSelect().
		Model(&db.StageStep{ID: stepID}).
		WherePK().
		Exists(ctx)
	if err != nil {
		log.Errorf("Error while checking step %d exists %#v", stepID, err)
		return false
	}
	return exists
}

// func findStage(ctx context.Context, dbConn *bun.DB, name string, pipelineFile string, stage *db.Stage) error {
// 	if err := dbConn.NewSelect().
// 		Model(stage).
// 		Where("name = ? and pipeline_file = ? ", name, pipelineFile).
// 		Scan(ctx); err != nil {
// 		return err
// 	}
// 	return nil
// }

// func findStep(ctx context.Context, dbConn *bun.DB, name string, stageID int, step *db.StageStep) error {
// 	if err := dbConn.NewSelect().
// 		Model(step).
// 		Where("name = ? and stage_id = ?", name, stageID).
// 		Scan(ctx); err != nil {
// 		return err
// 	}
// 	return nil
// }

func (h *Handler) fromDroneStatus(droneStatus string) db.Status {
	h.DatabaseConfig.Log.Infof("Drone Status %s", droneStatus)
	switch droneStatus {
	case drone.StatusError:
		return db.Error
	case drone.StatusFailing:
		return db.Error
	case drone.StatusKilled:
		return db.Killed
	case drone.StatusRunning:
		return db.Running
	case "success":
		return db.Success
	default:
		return db.None
	}
}
