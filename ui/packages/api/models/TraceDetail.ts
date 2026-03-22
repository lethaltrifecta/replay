/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ToolCapture } from './ToolCapture';
import type { TraceStep } from './TraceStep';
export type TraceDetail = {
    traceId?: string;
    steps?: Array<TraceStep>;
    toolCaptures?: Array<ToolCapture>;
};

