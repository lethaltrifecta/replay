/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { Experiment } from '../models/Experiment';
import type { ExperimentDetail } from '../models/ExperimentDetail';
import type { ExperimentReport } from '../models/ExperimentReport';
import type { GateCheckRequest } from '../models/GateCheckRequest';
import type { GateCheckResponse } from '../models/GateCheckResponse';
import type { GateStatusResponse } from '../models/GateStatusResponse';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class ExperimentsService {
    /**
     * List experiments (Gate Runs)
     * @param status
     * @param limit
     * @param offset
     * @returns Experiment List of experiments
     * @throws ApiError
     */
    public static listExperiments(
        status?: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled',
        limit: number = 20,
        offset?: number,
    ): CancelablePromise<Array<Experiment>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/experiments',
            query: {
                'status': status,
                'limit': limit,
                'offset': offset,
            },
            errors: {
                500: `Generic error response`,
            },
        });
    }
    /**
     * Get experiment details
     * @param id
     * @returns ExperimentDetail
     * @throws ApiError
     */
    public static getExperiment(
        id: string,
    ): CancelablePromise<ExperimentDetail> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/experiments/{id}',
            path: {
                'id': id,
            },
            errors: {
                404: `Generic error response`,
                500: `Generic error response`,
            },
        });
    }
    /**
     * Get canonical experiment report for review
     * @param id
     * @returns ExperimentReport
     * @throws ApiError
     */
    public static getExperimentReport(
        id: string,
    ): CancelablePromise<ExperimentReport> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/experiments/{id}/report',
            path: {
                'id': id,
            },
            errors: {
                404: `Generic error response`,
                500: `Generic error response`,
            },
        });
    }
    /**
     * Start a deployment gate check
     * @param requestBody
     * @returns GateCheckResponse Gate check started
     * @throws ApiError
     */
    public static createGateCheck(
        requestBody: GateCheckRequest,
    ): CancelablePromise<GateCheckResponse> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/gate/check',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Generic error response`,
                404: `Generic error response`,
                500: `Generic error response`,
                503: `Generic error response`,
            },
        });
    }
    /**
     * Get status of a gate check
     * @param id
     * @returns GateStatusResponse
     * @throws ApiError
     */
    public static getGateStatus(
        id: string,
    ): CancelablePromise<GateStatusResponse> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/gate/status/{id}',
            path: {
                'id': id,
            },
            errors: {
                404: `Generic error response`,
                500: `Generic error response`,
            },
        });
    }
}
