/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { AnalysisResult } from './AnalysisResult';
import type { ExperimentRun } from './ExperimentRun';
export type ExperimentReport = {
    experimentId?: string;
    baselineTraceId?: string;
    status?: string;
    verdict?: string;
    similarityScore?: number;
    tokenDelta?: number;
    latencyDelta?: number;
    analysis?: AnalysisResult;
    error?: string;
    runs?: Array<ExperimentRun>;
};

