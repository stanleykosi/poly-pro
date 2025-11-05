/**
 * @description
 * This page component renders the user sign-up interface.
 * It utilizes the pre-built `<SignUp>` component from Clerk, which handles
 * all the logic for new user registration, including email verification and
 * social sign-ups.
 *
 * Key features:
 * - Clerk Integration: Directly uses the `<SignUp>` component for a complete
 *   and secure registration flow.
 * - Centered Layout: The component is centered on the page, providing a
 *   user-friendly registration experience.
 *
 * @dependencies
 * - @clerk/nextjs: Provides the `<SignUp>` component.
 *
 * @notes
 * - The file path `[[...sign-up]]` is a convention used by Clerk with the Next.js
 *   App Router to correctly handle different steps and methods of the sign-up process.
 */
"use client"

import { SignUp } from '@clerk/nextjs'

export default function SignUpPage() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <SignUp />
    </div>
  )
}

