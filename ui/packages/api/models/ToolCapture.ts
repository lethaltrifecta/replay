/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type ToolCapture = {
    stepIndex?: number;
    toolName?: string;
    args?: Record<string, any>;
    result?: Record<string, any>;
    riskClass?: ToolCapture.riskClass;
    latencyMs?: number;
    error?: string;
};
export namespace ToolCapture {
    export enum riskClass {
        READ = 'read',
        WRITE = 'write',
        DESTRUCTIVE = 'destructive',
    }
}

