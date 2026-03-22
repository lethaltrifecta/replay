/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DriftDetails } from './DriftDetails';
export type DriftResult = {
    traceId?: string;
    baselineTraceId?: string;
    driftScore?: number;
    verdict?: DriftResult.verdict;
    details?: DriftDetails;
    createdAt?: string;
};
export namespace DriftResult {
    export enum verdict {
        PASS = 'pass',
        WARN = 'warn',
        FAIL = 'fail',
        PENDING = 'pending',
    }
}

