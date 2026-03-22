/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { VariantConfig } from './VariantConfig';
export type ExperimentRun = {
    id?: string;
    runType?: ExperimentRun.runType;
    traceId?: string;
    status?: string;
    error?: string;
    variantConfig?: VariantConfig;
    createdAt?: string;
};
export namespace ExperimentRun {
    export enum runType {
        BASELINE = 'baseline',
        VARIANT = 'variant',
    }
}

