/**
 * @description
 * This file serves as the main entry point for all TypeScript type definitions
 * within the frontend application. By exporting all types from this central "barrel" file,
 * we can ensure clean and consistent import paths across the entire codebase.
 *
 * For example, instead of importing from ` '@/types/market-types'`, other files
 * can simply import from `'@/types'`.
 *
 * Key features:
 * - Centralization: Provides a single source for all custom type definitions.
 * - Clean Imports: Simplifies import statements in other files.
 * - Discoverability: Makes it easy to find and browse all available custom types.
 */

// Export all types from the market-types file.
// As new type definition files are added, they should be exported here as well.
export * from './market-types'

