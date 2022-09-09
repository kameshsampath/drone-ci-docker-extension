import { createAsyncThunk, createSlice, current, PayloadAction } from '@reduxjs/toolkit';
import { AppThunk, RootState } from '../app/store';
import { getDockerDesktopClient } from '../utils';
import { Pipeline, PipelinesState, PipelineStatus, Stage, Status, StepCountPayload, StepPayload } from './types';
import * as _ from 'lodash';

const initialState: PipelinesState = {
  status: 'idle',
  rows: []
};

const ddClient = getDockerDesktopClient();

export const selectRows = (state: RootState) => state.pipelines.rows;
export const dataLoadStatus = (state: RootState) => state.pipelines.status;

function computePipelineStatus(state, pipelineId): PipelineStatus {
  const pipeline = _.find(state.rows, { id: pipelineId });

  //console.log('Pipeline ' + JSON.stringify(pipeline));
  if (pipeline) {
    const steps = pipeline.steps;

    const runningSteps = _.filter(steps, (s) => s.status?.toLowerCase() === 'start');

    const erroredSteps = _.filter(steps, (s) => s.status?.toLowerCase() === 'error');

    const allDoneSteps = _.filter(steps, (s) => s.status?.toLowerCase() === 'done');

    return {
      total: steps?.length,
      running: runningSteps?.length,
      error: erroredSteps?.length,
      done: allDoneSteps?.length
    };
  }
}

export const importPipelines = createAsyncThunk('pipelines/loadStages', async () => {
  const response = (await ddClient.extension.vm.service.get('/stages')) as Stage[];
  console.log('Loading pipelines from backend %s', response.length);
  const groupedStages = _.groupBy(response, 'pipelineFile');
  const pipelines = new Array<Pipeline>();
  for (const [key, value] of Object.entries(groupedStages)) {
    pipelines.push({
      pipelineFile: key,
      stages: value
    } as Pipeline);
  }
  return pipelines;
});

export const persistPipeline = createAsyncThunk('pipelines/persistPipeline', async (pipelineID: string) => {
  const idx = _.findIndex(selectRows, { id: pipelineID });
  if (idx != -1) {
    const pipeline = selectRows[idx];
    console.log('Persisting Pipeline %s', JSON.stringify(pipeline));
    // try {
    //   const response = await ddClient.extension.vm.service.post('/pipeline', pipeline);
    //   console.log('Saved pipelines' + JSON.stringify(response));
    // } catch (err) {
    //   console.error('Error Saving' + JSON.stringify(err));
    //   ddClient.desktopUI.toast.error(`Error saving pipelines ${err.message}`);
    // }
  }
});

export const savePipelines = (): AppThunk => async (_dispatch, getState) => {
  const currState = getState().pipelines;
  const pipelines = currState.rows;
  //console.log('Saving pipelines to backend ' + JSON.stringify(pipelines));
  if (pipelines?.keys.length > 0) {
    try {
      const response = await ddClient.extension.vm.service.post('/pipeline', pipelines);
      console.log('Saved pipelines' + JSON.stringify(response));
    } catch (err) {
      console.error('Error Saving' + JSON.stringify(err));
      ddClient.desktopUI.toast.error(`Error saving pipelines ${err.message}`);
    }
  }
};

export const pipelinesSlice = createSlice({
  name: 'pipelines',
  initialState,
  reducers: {
    loadStages: (state, action: PayloadAction<Pipeline[]>) => {
      state.status = 'loaded';
      const newRows = rowsFromPayload(action.payload);
      // console.log('Existing Rows ', JSON.stringify(state.rows));
      // console.log('New Rows ', JSON.stringify(newRows));
      state.rows = _.unionBy(state.rows, newRows, 'pipelineFile');
    },
    updateStep: (state, action: PayloadAction<StepPayload>) => {
      //console.log('Action::' + action.type + '::' + JSON.stringify(action.payload));
      // const { pipelineID, step } = action.payload;
      // const idx = _.findIndex(state.rows, { id: pipelineID });
      // if (idx != -1) {
      //   // console.log(' Update Found::' + idx + '::' + JSON.stringify(state.rows[idx]));
      //   const oldSteps = state.rows[idx].steps;
      //   const stepIdx = _.findIndex(oldSteps, { name: step.name });
      //   //console.log('Update Found Step::' + stepIdx + '::' + JSON.stringify(oldSteps));
      //   if (stepIdx != -1) {
      //     oldSteps[stepIdx] = step;
      //     state.rows[idx].steps = oldSteps;
      //     updatePipelineStatus(state, pipelineID);
      //   }
      // }
    },
    deleteSteps: (state, action: PayloadAction<StepPayload>) => {
      // //console.log("Action::" + action.type + "::" + action.payload);
      // const { pipelineID, step } = action.payload;
      // const idx = _.findIndex(state.rows, { id: pipelineID });
      // if (idx != -1) {
      //   const j = _.findIndex(state.rows[idx].steps, { name: step.name });
      //   state.rows[idx].steps.splice(j, 1);
      // }
    },
    updateStepCount: (state, action: PayloadAction<StepCountPayload>) => {
      const { pipelineID, status } = action.payload;
      const idx = _.findIndex(state.rows, { id: pipelineID });
      if (idx != -1) {
        // state.rows[idx].status.total = status.total;
        // state.rows[idx].status.done = status.done;
        // state.rows[idx].status.error = status.error;
        // state.rows[idx].status.running = status.running;
      }
    },
    pipelineStatus: (state, action: PayloadAction<string>) => {
      //console.log("Action::pipelineStatus::Payload" + action.payload);
      const pipelineID = action.payload;
      updatePipelineStatus(state, pipelineID);
    },
    removeStages: (state, action: PayloadAction<string[]>) => {
      const pipelineIds = action.payload;
      // console.log('Action::removePipelines::Payload' + JSON.stringify(pipelineIds));
      state.rows = _.remove(state.rows, (o) => !_.includes(pipelineIds, o.pipelineFile));
    },
    resetPipelineStatus: (state, action: PayloadAction<StepCountPayload>) => {
      const { pipelineID, status } = action.payload;
      const idx = _.findIndex(state.rows, { id: pipelineID });
      if (idx != -1) {
        // state.rows[idx].status.total = status.total;
        // state.rows[idx].status.error = status.error;
        // state.rows[idx].status.running = status.running;
        // state.rows[idx].status.done = status.done;
        //reset step statuses
        // state.rows[idx].steps.forEach((step) => {
        //   step.status = '0';
        // });
      }
      updatePipelineStatus(state, pipelineID);
    }
  },
  extraReducers: (builder) => {
    builder
      .addCase(importPipelines.pending, (state) => {
        state.status = 'loading';
        state.rows = [];
      })
      .addCase(importPipelines.fulfilled, (state, action) => {
        state.status = 'loaded';
        state.rows = rowsFromPayload(action.payload);
      })
      .addCase(importPipelines.rejected, (state) => {
        state.status = 'failed';
        state.rows = [];
      })
      .addCase(persistPipeline.fulfilled, () => {
        console.log('saved');
      })
      .addCase(persistPipeline.rejected, () => {
        console.log('save error');
      });
  }
});

export const {
  loadStages,
  pipelineStatus,
  updateStep,
  deleteSteps,
  removeStages,
  updateStepCount,
  resetPipelineStatus
} = pipelinesSlice.actions;

function updatePipelineStatus(state, pipelineId: string) {
  const status = computePipelineStatus(state, pipelineId);
  //console.log('Update Pipeline Status..' + JSON.stringify(status));
  const idx = _.findIndex(state.rows, { id: pipelineId });
  if (idx != -1) {
    state.rows[idx].status.error = status.error;
    state.rows[idx].status.running = status.running;
    state.rows[idx].status.done = status.done;
  }
}

function rowsFromPayload(payload: Pipeline[]) {
  console.log('Received Payload ' + JSON.stringify(payload));
  const rows = new Array<Pipeline>();
  payload.map((v) => {
    rows.push({
      pipelineFile: v.pipelineFile,
      stages: v.stages
    } as Pipeline);
  });
  return rows;
}

export default pipelinesSlice.reducer;
