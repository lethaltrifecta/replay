/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TraceDetail } from './TraceDetail';
export type TraceComparison = {
    baseline?: TraceDetail;
    candidate?: TraceDetail;
    diff?: {
        divergenceReason?: string;
        divergenceStepIndex?: number;
        similarityScore?: number;
    };
};

