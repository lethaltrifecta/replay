/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TraceComparison } from '../models/TraceComparison';
import type { TraceDetail } from '../models/TraceDetail';
import type { TraceSummary } from '../models/TraceSummary';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class TracesService {
    /**
     * List replayed traces
     * @param model
     * @param provider
     * @param limit
     * @param offset
     * @returns TraceSummary
     * @throws ApiError
     */
    public static listTraces(
        model?: string,
        provider?: string,
        limit: number = 20,
        offset?: number,
    ): CancelablePromise<Array<TraceSummary>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/traces',
            query: {
                'model': model,
                'provider': provider,
                'limit': limit,
                'offset': offset,
            },
        });
    }
    /**
     * Get trace details (steps)
     * @param traceId
     * @returns TraceDetail
     * @throws ApiError
     */
    public static getTrace(
        traceId: string,
    ): CancelablePromise<TraceDetail> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/traces/{traceId}',
            path: {
                'traceId': traceId,
            },
            errors: {
                404: `Generic error response`,
            },
        });
    }
    /**
     * Side-by-side trace comparison
     * @param baselineTraceId
     * @param candidateTraceId
     * @returns TraceComparison
     * @throws ApiError
     */
    public static compareTraces(
        baselineTraceId: string,
        candidateTraceId: string,
    ): CancelablePromise<TraceComparison> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/compare/{baselineTraceId}/{candidateTraceId}',
            path: {
                'baselineTraceId': baselineTraceId,
                'candidateTraceId': candidateTraceId,
            },
            errors: {
                404: `Generic error response`,
            },
        });
    }
}
