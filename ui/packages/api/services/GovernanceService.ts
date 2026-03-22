/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { Baseline } from '../models/Baseline';
import type { DriftResult } from '../models/DriftResult';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class GovernanceService {
    /**
     * List all approved baselines
     * @returns Baseline A list of baselines
     * @throws ApiError
     */
    public static listBaselines(): CancelablePromise<Array<Baseline>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/baselines',
            errors: {
                500: `Generic error response`,
            },
        });
    }
    /**
     * Mark a trace as a baseline
     * @param traceId
     * @param requestBody
     * @returns Baseline Baseline created
     * @throws ApiError
     */
    public static createBaseline(
        traceId: string,
        requestBody?: {
            name?: string;
            description?: string;
        },
    ): CancelablePromise<Baseline> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/baselines/{traceId}',
            path: {
                'traceId': traceId,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Generic error response`,
                404: `Generic error response`,
            },
        });
    }
    /**
     * Unmark a trace as baseline
     * @param traceId
     * @returns void
     * @throws ApiError
     */
    public static deleteBaseline(
        traceId: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/baselines/{traceId}',
            path: {
                'traceId': traceId,
            },
            errors: {
                404: `Generic error response`,
            },
        });
    }
    /**
     * List drift results (Drift Inbox)
     * @param limit
     * @param offset
     * @returns DriftResult A list of drift results
     * @throws ApiError
     */
    public static listDriftResults(
        limit: number = 20,
        offset?: number,
    ): CancelablePromise<Array<DriftResult>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/drift-results',
            query: {
                'limit': limit,
                'offset': offset,
            },
        });
    }
    /**
     * Get drift details for a specific trace
     * @param traceId
     * @param baselineTraceId
     * @returns DriftResult Drift result detail
     * @throws ApiError
     */
    public static getDriftResult(
        traceId: string,
        baselineTraceId?: string,
    ): CancelablePromise<DriftResult> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/drift-results/{traceId}',
            path: {
                'traceId': traceId,
            },
            query: {
                'baselineTraceId': baselineTraceId,
            },
            errors: {
                400: `Generic error response`,
                404: `Generic error response`,
            },
        });
    }
}
