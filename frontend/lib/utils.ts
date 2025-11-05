/**
 * @description
 * This utility file, a standard part of the Shadcn UI setup, provides a helper
 * function for conditionally combining CSS classes.
 *
 * Key features:
 * - cn function: A wrapper around `clsx` and `tailwind-merge` that allows for
 *   building dynamic and conflict-free class strings for components. This is
 *   essential for creating reusable components whose styles can be extended.
 *
 * @dependencies
 * - clsx: For constructing class name strings conditionally.
 * - tailwind-merge: For intelligently merging Tailwind CSS classes without style conflicts.
 */

import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

