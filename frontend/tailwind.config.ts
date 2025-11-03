import type { Config } from 'tailwindcss'

/**
 * @description
 * This is the Tailwind CSS configuration file for the Poly-Pro Analytics frontend.
 * It defines the design system, including the color palette, fonts, and other theme settings.
 *
 * Key features:
 * - Content Scanning: Configured to scan all relevant files in the `app` and `components` directories for Tailwind utility classes.
 * - Theme Extensibility: Prepared for future extension with custom colors, fonts, and spacing as per the project's design system.
 *
 * @notes
 * - The theme will be extended in a later step to match the dark mode style specified in the technical docs.
 */
const config: Config = {
  content: [
    './pages/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
    './app/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-conic':
          'conic-gradient(from 180deg at 50% 50%, var(--tw-gradient-stops))',
      },
    },
  },
  plugins: [],
}

export default config

