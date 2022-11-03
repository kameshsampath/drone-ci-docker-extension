import React, { useEffect } from 'react';
import { useState } from 'react';
import { createSearchParams, Link as RouterLink, LinkProps as RouterLinkProps } from 'react-router-dom';
import { Stack } from '@mui/material';
import RemovePipelineDialog from './dialogs/RemovePipelineDialog';
import StopPipelineDialog from './dialogs/StopPipelineDialog';
import { useSelector } from 'react-redux/es/hooks/useSelector';
import { RootState } from '../app/store';
import { selectPipelineStatus } from '../features/pipelinesSlice';
import { actionButtons } from './common/actions';

export const PipelineRowActions = (props: { workspacePath: string; pipelineFile: string; logHandler; openHandler }) => {
  //!!!IMPORTANT - pass the location query params
  const [runViewURL, setRunViewURL] = useState({});
  const { pipelineFile, workspacePath } = props;
  const [removeConfirm, setRemoveConfirm] = useState(false);
  const [stopConfirm, setStopConfirm] = useState(false);

  const pipelineStatus = useSelector((state: RootState) => selectPipelineStatus(state, pipelineFile));

  /* Handlers */
  const handleDeletePipelines = () => {
    setRemoveConfirm(true);
  };

  const handleRemoveDialogClose = () => {
    setRemoveConfirm(false);
  };

  const handleStopPipeline = () => {
    setStopConfirm(true);
  };

  const handleStopPipelineDialogClose = () => {
    setStopConfirm(false);
  };


  useEffect(() => {
    const url = {
      pathname: '/run',
      search: `?${createSearchParams({
        file: pipelineFile,
        runPipeline: 'true'
      })}`
    };
    setRunViewURL(url);
  }, [pipelineFile]);

  const NavigateToRunView = React.forwardRef<any, Omit<RouterLinkProps, 'to'>>((props, ref) => (
    <RouterLink
      ref={ref}
      to={runViewURL}
      {...props}
      role={undefined}
    />
  ));

  return (
    <Stack
      direction="row"
      spacing={2}
    >
     {actionButtons(pipelineStatus,workspacePath,handleStopPipeline,handleDeletePipelines,NavigateToRunView)}
     {stopConfirm && (
        <StopPipelineDialog
          open={stopConfirm}
          pipelineFile={pipelineFile}
          onClose={handleStopPipelineDialogClose}
        />
      )}
      {removeConfirm && (
        <RemovePipelineDialog
          open={removeConfirm}
          selectedToRemove={[pipelineFile]}
          onClose={handleRemoveDialogClose}
        />
      )}
    </Stack>
  );
};