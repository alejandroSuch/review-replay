import { describe, expect, mock, test } from 'claude-code/testing'
import type { On } from 'claude-code'

import { needsGhLookup, parseVerdict } from './register'

const PANE = {
  component: 'Pane',
  requestId: 'review-replay',
  props: {
    title: 'Review replay',
    isFocused: true,
    bodyColumns: 100,
    placement: 'dock',
    scroll: { offset: 0, bodyRows: 40 },
    view: {},
  },
} as const

const BUNDLE = {
  pr: 'acme/app#7',
  url: 'https://github.com/acme/app/pull/7',
  prAuthor: 'dev',
  commits: 3,
  historyRewritten: false,
  items: [
    {
      commentId: 1,
      origin: 'inline',
      author: 'rev',
      path: 'src/a.go',
      line: 12,
      body: 'Rename this',
      htmlUrl: 'https://github.com/acme/app/pull/7#r1',
      system: 'SYSTEM',
      user: 'USER',
    },
    {
      commentId: 2,
      origin: 'inline',
      author: 'rev',
      path: 'src/b.go',
      line: 3,
      body: 'Missing nil check',
      htmlUrl: 'https://github.com/acme/app/pull/7#r2',
      rule: { name: 'no-signal', status: 'pending', confidence: 'high', rationale: 'Nothing touched it.' },
    },
  ],
}

const USAGE = { input_tokens: 1, output_tokens: 1, cache_creation_input_tokens: 0, cache_read_input_tokens: 0 }

/** Stands in for gh, the CLI and the model beneath the plugin. */
function world(on: On) {
  const ran: string[][] = []
  on('process.run', async (_$, e) => {
    ran.push([...e.argv])
    if (e.argv[0] === 'gh') return { value: { exitCode: 0, stdout: 'tok\n', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
    return { value: { exitCode: 0, stdout: JSON.stringify(BUNDLE), stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('model.complete', async () => ({
    value: {
      isAnswered: true,
      text: '{"status":"addressed","confidence":"high","rationale":"Renamed.","draftReply":"Done, renamed."}',
      usage: USAGE,
    },
  }))
  on('ui.open', async () => ({ value: { isPlaced: true } }))
  return ran
}

describe('parseVerdict', () => {
  test('reads fenced JSON', async () => {
    const v = parseVerdict('```json\n{"status":"partial","confidence":"medium","rationale":"half"}\n```', false)
    expect(v?.status).toBe('partial')
    expect(v?.confidence).toBe('medium')
  })
  test('finds JSON inside prose', async () => {
    expect(parseVerdict('Sure: {"status":"pending","confidence":"low","rationale":"x"} ok', false)?.status).toBe('pending')
  })
  test('refuses unknown statuses', async () => {
    expect(parseVerdict('{"status":"done","confidence":"high","rationale":""}', false)).toBeUndefined()
  })
  test('strips SHAs from replies on a force-pushed branch', async () => {
    const v = parseVerdict('{"status":"addressed","confidence":"high","rationale":"","draftReply":"Fixed in abc1234 here"}', true)
    expect(v?.draftReply).toBe('Fixed in here')
  })
})

test('a bare number or nothing goes through gh', async () => {
  expect(needsGhLookup('')).toBe(true)
  expect(needsGhLookup('#42')).toBe(true)
  expect(needsGhLookup('acme/app#42')).toBe(false)
  expect(needsGhLookup('https://github.com/acme/app/pull/42')).toBe(false)
})

test('/review-pane fetches, classifies and draws the table on every surface', async ($, on) => {
  const clock = mock.clock(on)
  const ran = world(on)
  const { text } = await $.command.run({
    command: 'review-pane',
    args: 'acme/app#7',
    origin: { kind: 'composer' },
    presentation: { isFullscreen: true, columns: 160 },
  })
  expect(text).toContain('acme/app#7')
  await clock.advance(10)
  expect(ran.find(argv => argv.includes('--emit-prompts'))).toEqual(['review-replay', 'acme/app#7', '--emit-prompts'])

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ plugin: 'review-replay', surface, ...PANE })
    expect(await ui.find({ type: 'Text', text: /✓ 1/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /✗ 1/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /src\/a\.go:12/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /Done, renamed\./ })).toBeDefined()
    expect(await ui.find({ key: 'copy' })).toBeDefined()
    await ui.press({ key: 'next' })
    expect(await ui.find({ type: 'Text', text: /rule:no-signal/ })).toBeDefined()
    expect(await ui.find({ key: 'copy' })).toBeUndefined()
    await ui.press({ key: 'prev' })
    await ui.unmount()
  }
})
