/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class SystemService {
    /**
     * Health check
     * @returns any
     * @throws ApiError
     */
    public static getHealth(): CancelablePromise<{
        status?: 'ok' | 'degraded';
    }> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/health',
        });
    }
}
