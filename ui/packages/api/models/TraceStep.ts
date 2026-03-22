/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { PromptContent } from './PromptContent';
export type TraceStep = {
    spanId?: string;
    stepIndex?: number;
    provider?: string;
    model?: string;
    prompt?: PromptContent;
    completion?: string;
    promptTokens?: number;
    completionTokens?: number;
    latencyMs?: number;
};

