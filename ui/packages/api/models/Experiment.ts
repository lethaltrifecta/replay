/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { VariantConfig } from './VariantConfig';
export type Experiment = {
    id?: string;
    name?: string;
    baselineTraceId?: string;
    status?: string;
    progress?: number;
    threshold?: number;
    verdict?: string;
    variantConfig?: VariantConfig;
    createdAt?: string;
    completedAt?: string;
};

