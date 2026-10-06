import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The browser sweeps in scripts/e2e drive the real workspace and catch what only
// a browser can. What they cannot do is tell you which rule was wrong when a
// finding comes out in the wrong words — so the editor's pure logic is tested
// here, where a failure names the rule.
export default defineConfig({
  // The same JSX transform the app is built with, so a component under test is
  // the component that ships.
  plugins: [react()],
  // The app's tsconfig says react-jsx; a test file is compiled by esbuild here
  // and has to be told the same, or a component under test compiles against a
  // React the test never imported.
  esbuild: { jsx: 'automatic' },
  test: {
    setupFiles: ['src/test/setup.ts'],
    // A dialog is tested where a dialog lives; the rules that write a sentence
    // do not need a document, and starting one for them wastes a second per run.
    // That split used to be one `environmentMatchGlobs` line, which Vitest 4
    // dropped; two projects are what it points at instead. They inherit the
    // plugins and the setup file from this file, so only the two things that
    // differ are written out. A single `environment: 'jsdom'` would be shorter
    // and would throw the saved second away on every rules-only file, so the
    // split stays.
    projects: [
      {
        test: {
          name: 'rules',
          environment: 'node',
          include: ['src/**/*.test.ts'],
        },
      },
      {
        test: {
          name: 'components',
          environment: 'jsdom',
          include: ['src/**/*.test.tsx'],
        },
      },
    ],
  },
})
