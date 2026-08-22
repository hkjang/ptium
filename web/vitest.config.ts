import { defineConfig } from 'vitest/config'

// The browser sweeps in scripts/e2e drive the real workspace and catch what only
// a browser can. What they cannot do is tell you which rule was wrong when a
// finding comes out in the wrong words — so the editor's pure logic is tested
// here, where a failure names the rule.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
})
