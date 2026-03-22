/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ReplayRequestHeaders } from './ReplayRequestHeaders';
export type GateCheckRequest = {
    baselineTraceId: string;
    model: string;
    provider?: string;
    threshold?: number;
    temperature?: number;
    topP?: number;
    maxTokens?: number;
    requestHeaders?: ReplayRequestHeaders;
};

