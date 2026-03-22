/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { PromptMessage } from './PromptMessage';
export type PromptContent = {
    messages?: Array<PromptMessage>;
    tools?: Array<Record<string, any>>;
    /**
     * Arbitrary tool-choice payload captured from the source trace.
     */
    tool_choice?: any;
};

