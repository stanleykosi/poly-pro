/**
 * @description
 * This file configures PostCSS for the Next.js frontend.
 * It is essential for processing CSS, particularly for integrating Tailwind CSS.
 *
 * Key features:
 * - Tailwind CSS: Enables the use of the Tailwind CSS utility-first framework.
 * - Autoprefixer: Automatically adds vendor prefixes to CSS properties for better browser compatibility.
 */
module.exports = {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
}

