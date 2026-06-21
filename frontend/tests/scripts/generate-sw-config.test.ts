/**
 * Tests for scripts/generate-sw-config.js
 *
 * The generator is a plain Node.js ES module invoked via `node` — not part of
 * the Vite/TypeScript build. We exercise it here by invoking it in a child
 * process (to isolate env vars) and asserting the output file content.
 */

import { execSync } from 'child_process'
import { mkdtempSync, readFileSync, rmSync } from 'fs'
import { join, resolve } from 'path'
import { tmpdir } from 'os'

// Path to the script under test (resolved relative to this test file)
const SCRIPT_PATH = resolve(__dirname, '../../scripts/generate-sw-config.js')

function runScript(
  env: Record<string, string | undefined>,
  outDir: string
): string {
  // The script writes to resolve(__dirname, '..', 'public').
  // Copy it into tmpDir/scripts/ so __dirname → tmpDir/scripts
  // and the output becomes tmpDir/public/firebase-messaging-sw-config.js.
  const { execFileSync } = require('child_process')
  const { mkdirSync, copyFileSync } = require('fs')
  const tmpScriptsDir = join(outDir, 'scripts')
  mkdirSync(tmpScriptsDir, { recursive: true })
  mkdirSync(join(outDir, 'public'), { recursive: true })
  copyFileSync(SCRIPT_PATH, join(tmpScriptsDir, 'generate-sw-config.js'))

  execFileSync('node', [join(tmpScriptsDir, 'generate-sw-config.js')], {
    env: { ...process.env, ...env },
    cwd: outDir,
    encoding: 'utf8',
  })

  return readFileSync(
    join(outDir, 'public', 'firebase-messaging-sw-config.js'),
    'utf8'
  )
}

describe('generate-sw-config.js', () => {
  let tmpDir: string

  beforeEach(() => {
    tmpDir = mkdtempSync(join(tmpdir(), 'sw-config-test-'))
  })

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true })
  })

  it('writes self.FIREBASE_* globals from VITE_FIREBASE_* env vars', () => {
    const content = runScript(
      {
        VITE_FIREBASE_API_KEY: 'test-api-key',
        VITE_FIREBASE_AUTH_DOMAIN: 'test.firebaseapp.com',
        VITE_FIREBASE_PROJECT_ID: 'test-project',
        VITE_FIREBASE_STORAGE_BUCKET: '',
        VITE_FIREBASE_MESSAGING_SENDER_ID: '123456789',
        VITE_FIREBASE_APP_ID: '1:123:web:abc',
        FIREBASE_CONFIG_REQUIRED: undefined,
      },
      tmpDir
    )

    expect(content).toContain('self.FIREBASE_API_KEY = "test-api-key";')
    expect(content).toContain(
      'self.FIREBASE_AUTH_DOMAIN = "test.firebaseapp.com";'
    )
    expect(content).toContain('self.FIREBASE_PROJECT_ID = "test-project";')
    expect(content).toContain(
      'self.FIREBASE_MESSAGING_SENDER_ID = "123456789";'
    )
    expect(content).toContain('self.FIREBASE_APP_ID = "1:123:web:abc";')
  })

  it('writes empty strings when env vars are absent (graceful degradation)', () => {
    const content = runScript(
      {
        VITE_FIREBASE_API_KEY: undefined,
        FIREBASE_CONFIG_REQUIRED: undefined,
      },
      tmpDir
    )

    expect(content).toContain('self.FIREBASE_API_KEY = "";')
    expect(content).toContain('self.FIREBASE_MESSAGING_SENDER_ID = "";')
  })

  it('escapes special characters in values via JSON.stringify', () => {
    const content = runScript(
      {
        VITE_FIREBASE_API_KEY: 'key-with-"quotes"-and-\\backslash',
        VITE_FIREBASE_MESSAGING_SENDER_ID: '123',
        FIREBASE_CONFIG_REQUIRED: undefined,
      },
      tmpDir
    )

    // JSON.stringify will escape the double quotes and backslash
    expect(content).toContain(
      'self.FIREBASE_API_KEY = "key-with-\\"quotes\\"-and-\\\\backslash";'
    )
  })

  it('does NOT write FIREBASE_VAPID_KEY (intentionally omitted — only used by foreground bundle)', () => {
    const content = runScript(
      {
        VITE_FIREBASE_VAPID_KEY: 'some-vapid-key',
        FIREBASE_CONFIG_REQUIRED: undefined,
      },
      tmpDir
    )

    expect(content).not.toContain('FIREBASE_VAPID_KEY')
  })

  it('exits non-zero when FIREBASE_CONFIG_REQUIRED=true and required vars are missing', () => {
    const { mkdirSync, copyFileSync } = require('fs')
    const tmpScriptsDir = join(tmpDir, 'scripts')
    mkdirSync(tmpScriptsDir, { recursive: true })
    mkdirSync(join(tmpDir, 'public'), { recursive: true })
    copyFileSync(SCRIPT_PATH, join(tmpScriptsDir, 'generate-sw-config.js'))

    expect(() => {
      execSync(`node ${join(tmpScriptsDir, 'generate-sw-config.js')}`, {
        env: {
          ...process.env,
          FIREBASE_CONFIG_REQUIRED: 'true',
          VITE_FIREBASE_API_KEY: '',
          VITE_FIREBASE_MESSAGING_SENDER_ID: '',
        },
        cwd: tmpDir,
        encoding: 'utf8',
      })
    }).toThrow()
  })

  it('succeeds without Firebase vars when FIREBASE_CONFIG_REQUIRED is not set (PR test build)', () => {
    const content = runScript(
      {
        VITE_FIREBASE_API_KEY: undefined,
        FIREBASE_CONFIG_REQUIRED: undefined,
      },
      tmpDir
    )

    // Should produce empty-string globals without throwing
    expect(content).toContain('self.FIREBASE_API_KEY = "";')
  })
})
