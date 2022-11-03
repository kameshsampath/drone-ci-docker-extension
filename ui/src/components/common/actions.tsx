/* eslint-disable @typescript-eslint/no-explicit-any */
import { IconButton, Tooltip } from "@mui/material"
import { Status } from "../../features/types"
import { vscodeURI } from "../../utils"
import PlayCircleFilledWhiteIcon from '@mui/icons-material/PlayCircleFilledWhite';
import StopCircleIcon from '@mui/icons-material/StopCircle';
import DeleteIcon from '@mui/icons-material/Delete';

const run = (pipelineStatus: Status, handleRunPipeline, NavigateToRunView) => {
	if (NavigateToRunView) {
		return (
			<IconButton component={NavigateToRunView}
				disabled={pipelineStatus === Status.RUNNING}>
				<PlayCircleFilledWhiteIcon color={pipelineStatus === Status.RUNNING ? "disabled" : "info"} />
			</IconButton>
		)
	} else if (handleRunPipeline) {
		return (
			<IconButton onClick={handleRunPipeline}
				disabled={pipelineStatus === Status.RUNNING}>
				<PlayCircleFilledWhiteIcon color={pipelineStatus === Status.RUNNING ? "disabled" : "info"} />
			</IconButton>
		)
	}
}

export const actionButtons = (
	pipelineStatus: any,
	workspacePath: any,
	handleStopPipeline: any,
	handleDeletePipelines: any,
	NavigateToRunView?: any,
	handleRunPipeline?: any) => {
	return (
		<>
			<Tooltip title="Run Pipeline">
				<span>
					{run(pipelineStatus, handleRunPipeline, NavigateToRunView)}
				</span>
			</Tooltip>
			<Tooltip title="Open in VS Code">
				<IconButton
					aria-label="edit in vscode"
					color="primary"
					href={vscodeURI(workspacePath)}>
					<img
						src={process.env.PUBLIC_URL + '/images/vscode.png'}
						width="16"
					/>
				</IconButton>
			</Tooltip>
			<Tooltip title="Stop Pipeline">
				<span>
					<IconButton
						onClick={handleStopPipeline}
						disabled={pipelineStatus != Status.RUNNING}>
						<StopCircleIcon color={pipelineStatus != Status.RUNNING ? "disabled" : 'primary'} />
					</IconButton>
				</span>
			</Tooltip>
			<Tooltip title="Remove Pipeline">
				<span>
					<IconButton
						onClick={handleDeletePipelines}
						disabled={pipelineStatus === Status.RUNNING}>
						<DeleteIcon color={pipelineStatus === Status.RUNNING ? "disabled" : "error"} />
					</IconButton>
				</span>
			</Tooltip>
		</>
	)
}