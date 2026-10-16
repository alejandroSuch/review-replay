import { atom, read, update } from 'claude-code'
import type { EngineInterface as Engine, Register } from 'claude-code'

import type { Confidence, Row, Run, Status } from '../types'

const PANE = 'review-replay'
const TITLE = 'Review replay'
const PARALLEL = 4

const EMPTY: Run = {
  phase: 'idle',
  pr: '',
  url: '',
  prAuthor: '',
  commits: 0,
  historyRewritten: false,
  rows: [],
}
export const run = atom({ plugin: 'review-replay', key: 'run' } as const, EMPTY)
const selected = atom({ plugin: 'review-replay', key: 'selected' } as const, 0)

/** What `review-replay --emit-prompts` prints (internal/pipeline/prompts.go). */
type Bundle = {
  pr: string
  url: string
  prAuthor: string
  commits: number
  historyRewritten: boolean
  items: Array<{
    commentId: number
    origin: Row['origin']
    author: string
    path?: string
    line?: number
    body: string
    htmlUrl: string
    reviewState?: string
    rule?: { name: string; status: Status; confidence: Confidence; rationale: string }
    system?: string
    user?: string
  }>
}

type Verdict = Pick<Row, 'status' | 'confidence' | 'rationale' | 'draftReply'>

const STATUSES: readonly Status[] = ['addressed', 'partial', 'pending', 'needs-discussion']
const CONFIDENCES: readonly Confidence[] = ['high', 'medium', 'low']

/** Mirrors classifier.parseClassification: fenced or bare JSON, else the first {...}. */
export function parseVerdict(text: string, historyRewritten: boolean): Verdict | undefined {
  const trimmed = text.trim().replace(/^```(?:json)?/, '').replace(/```$/, '').trim()
  const candidates = [trimmed]
  const start = trimmed.indexOf('{')
  const end = trimmed.lastIndexOf('}')
  if (start >= 0 && end > start) candidates.push(trimmed.slice(start, end + 1))
  for (const candidate of candidates) {
    let parsed: any
    try {
      parsed = JSON.parse(candidate)
    } catch {
      continue
    }
    if (!STATUSES.includes(parsed?.status) || !CONFIDENCES.includes(parsed?.confidence)) return undefined
    let draftReply: string | undefined = typeof parsed.draftReply === 'string' ? parsed.draftReply : undefined
    // A force-pushed branch may have orphaned any SHA the model cites.
    if (draftReply && historyRewritten) {
      draftReply = draftReply.replace(/\b[0-9a-f]{7,40}\b/g, '').split(/\s+/).filter(Boolean).join(' ')
    }
    return {
      status: parsed.status,
      confidence: parsed.confidence,
      rationale: String(parsed.rationale ?? ''),
      draftReply: draftReply || undefined,
    }
  }
  return undefined
}

/** `owner/repo#N` and full URLs go to the CLI as given; a bare number or nothing asks gh. */
export function needsGhLookup(arg: string): boolean {
  return arg === '' || /^#?\d+$/.test(arg)
}

type Config = { binary: string; model: string }

let generation = 0

async function fail($: Engine, error: string) {
  await update($, run, (r): Run => ({ ...r, phase: 'error', error }))
}


async function review($: Engine, arg: string, cfg: Config) {
  const { binary, model } = cfg
  const mine = ++generation
  const stale = () => mine !== generation
  await update($, selected, () => 0)
  await update($, run, () => ({ ...EMPTY, phase: 'fetching', pr: arg || 'current branch' }))

  let pr = arg
  if (needsGhLookup(arg)) {
    const view = await $.process.run(['gh', 'pr', 'view', ...(arg ? [arg.replace('#', '')] : []), '--json', 'url', '-q', '.url'])
    if (view.exitCode !== 0) return fail($, `gh could not find the PR: ${view.stderr.trim() || 'no PR for this branch'}`)
    pr = view.stdout.trim()
  }

  const token = await $.process.run(['gh', 'auth', 'token'])
  if (token.exitCode !== 0) return fail($, 'gh is not logged in. Run `gh auth login` first.')

  let out
  try {
    out = await $.process.run([binary, pr, '--emit-prompts'], {
      env: { GITHUB_TOKEN: token.stdout.trim() },
      timeoutMs: 5 * 60_000,
    })
  } catch (err) {
    return fail($, `Could not run ${binary}: ${String(err)}. Set the binary path with /config.`)
  }
  if (stale()) return
  if (out.exitCode !== 0) return fail($, out.stderr.trim() || `${binary} exited ${out.exitCode}`)

  let bundle: Bundle
  try {
    bundle = JSON.parse(out.stdout)
  } catch {
    return fail($, `${binary} printed no JSON. Is it a build with --emit-prompts?`)
  }

  const rows: Row[] = bundle.items.map(item => ({
    commentId: item.commentId,
    origin: item.origin,
    author: item.author,
    path: item.path,
    line: item.line,
    body: item.body,
    htmlUrl: item.htmlUrl,
    reviewState: item.reviewState,
    ...(item.rule && {
      status: item.rule.status,
      confidence: item.rule.confidence,
      rationale: item.rule.rationale,
      source: `rule:${item.rule.name}`,
    }),
  }))
  await update($, run, () => ({
    phase: 'classifying',
    pr: bundle.pr,
    url: bundle.url,
    prAuthor: bundle.prAuthor,
    commits: bundle.commits,
    historyRewritten: bundle.historyRewritten,
    rows,
  }))

  const queue = bundle.items.filter(item => !item.rule && item.user)
  const worker = async () => {
    for (let item = queue.shift(); item; item = queue.shift()) {
      const answer = await $.model.complete({
        model,
        system: item.system,
        prompt: item.user!,
        maxTokens: 800,
        effort: 'low',
        timeoutMs: 120_000,
      })
      if (stale()) return
      const verdict = answer.isAnswered ? parseVerdict(answer.text, bundle.historyRewritten) : undefined
      const patch: Partial<Row> = verdict
        ? { ...verdict, source: 'model' }
        : {
            status: 'needs-discussion',
            confidence: 'low',
            rationale: answer.isAnswered
              ? 'The model reply could not be parsed; flagged for human review.'
              : `The model call failed (${answer.reason}).`,
            source: 'error',
          }
      const id = item.commentId
      await update($, run, r => ({ ...r, rows: r.rows.map(row => (row.commentId === id ? { ...row, ...patch } : row)) }))
    }
  }
  await Promise.all(Array.from({ length: PARALLEL }, worker))
  if (!stale()) await update($, run, (r): Run => ({ ...r, phase: 'done' }))
}

export const register: Register = (on, options) => {
  const cfg: Config = { binary: String(options.binary ?? 'review-replay'), model: String(options.model ?? 'haiku') }

  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'review-pane',
      description: 'Check which PR review comments were addressed, in a pane',
      argumentHint: '[PR URL | owner/repo#N | number]',
    })
    return next(e)
  })

  on('command.run', { command: 'review-pane' }, async ($, e) => {
    const arg = e.args.trim()
    const current = await read($, run)
    await $.ui.open({ id: PANE, title: TITLE })
    if (!arg && current.phase !== 'idle') return { text: 'Review replay pane opened.' }
    // The run outlives this command: hand it to a timer.
    $.clock.after(1, () => void review($, arg, cfg).catch(err => fail($, String(err))))
    return { text: `Checking ${arg || 'the PR for this branch'} in the Review replay pane.` }
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text, Button, Link } = $.ui.resolve(e)
    const r = await read($, run)
    const at = await read($, selected)
    const cols = Math.max(40, e.props.bodyColumns ?? 80)

    if (r.phase === 'idle') {
      return <Text dimColor>Run /review-pane with a PR URL, owner/repo#N, or nothing for this branch's PR.</Text>
    }
    if (r.phase === 'error') {
      return (
        <Box flexDirection="column">
          <Text color="red">{r.error}</Text>
        </Box>
      )
    }
    if (r.phase === 'fetching') return <Text dimColor>Fetching {r.pr} from GitHub…</Text>

    const counts = STATUSES.map(s => [s, r.rows.filter(row => row.status === s).length] as const).filter(([, n]) => n > 0)
    const waiting = r.rows.filter(row => !row.status).length
    const row = r.rows[Math.min(at, r.rows.length - 1)]
    const move = (by: number) => () =>
      void update($, selected, i => Math.max(0, Math.min(r.rows.length - 1, i + by)))
    const placeWidth = Math.min(Math.floor(cols / 3), Math.max(...r.rows.map(one => short(one).length)))

    return (
      <Box flexDirection="column">
        <Box flexDirection="row" gap={2}>
          <Text bold wrap="truncate">
            {r.pr}
          </Text>
          {counts.map(([s, n]) => (
            <Text color={COLOR[s]}>
              {MARK[s]} {n}
            </Text>
          ))}
          {waiting > 0 && <Text dimColor>{waiting} classifying…</Text>}
        </Box>
        <Text> </Text>
        {r.rows.map((one, i) => (
          <Box key={`row-${one.commentId}`} flexDirection="row" gap={1}>
            <Box width={1} flexShrink={0}>
              <Text>{i === at ? '›' : ' '}</Text>
            </Box>
            <Box width={1} flexShrink={0}>
              <Text color={one.status ? COLOR[one.status] : undefined} dimColor={!one.status}>
                {one.status ? MARK[one.status] : '·'}
              </Text>
            </Box>
            <Box width={placeWidth} flexShrink={0}>
              <Text wrap="truncate-start" bold={i === at} dimColor={i !== at}>
                {short(one)}
              </Text>
            </Box>
            <Box flexGrow={1} flexShrink={1}>
              <Text wrap="truncate-end" bold={i === at} dimColor={i !== at}>
                {firstLine(one.body)}
              </Text>
            </Box>
          </Box>
        ))}
        <Text> </Text>
        {row && (
          <Box flexDirection="column" borderStyle="round" borderDimColor paddingX={1}>
            <Text>
              {row.status ? (
                <Text bold color={COLOR[row.status]}>
                  {MARK[row.status]} {row.status}
                </Text>
              ) : (
                <Text dimColor>classifying…</Text>
              )}
              {row.status && (
                <Text dimColor>
                  {' '}
                  · {row.confidence} confidence · {row.source}
                </Text>
              )}
            </Text>
            <Text dimColor wrap="wrap">
              {row.author} · {where(row)}
            </Text>
            <Text> </Text>
            <Text wrap="wrap">{clip(row.body, 600)}</Text>
            {row.rationale && (
              <>
                <Text> </Text>
                <Text dimColor>Why</Text>
                <Text wrap="wrap">{row.rationale}</Text>
              </>
            )}
            {row.draftReply && (
              <>
                <Text> </Text>
                <Text dimColor>Draft reply</Text>
                <Text wrap="wrap" italic>
                  {row.draftReply}
                </Text>
              </>
            )}
            <Text> </Text>
            <Link href={row.htmlUrl} label="Open on GitHub" />
          </Box>
        )}
        <Box flexDirection="row" gap={2}>
          <Button key="prev" label="Prev" hotkey="k" onPress={move(-1)} />
          <Button key="next" label="Next" hotkey="j" onPress={move(1)} />
          {row?.draftReply && (
            <Button
              key="copy"
              label="Copy reply"
              hotkey="c"
              onPress={press => void $.ui.copy({ text: row.draftReply!, surface: press.surface })}
            />
          )}
          {r.phase === 'done' && (
            <Button key="rerun" label="Re-run" hotkey="r" onPress={() => void review($, r.url, cfg).catch(err => fail($, String(err)))} />
          )}
        </Box>
      </Box>
    )
  })
}

const COLOR: Record<Status, string> = {
  addressed: 'green',
  partial: 'yellow',
  pending: 'red',
  'needs-discussion': 'magenta',
}
const MARK: Record<Status, string> = {
  addressed: '✓',
  partial: '◐',
  pending: '✗',
  'needs-discussion': '?',
}

function where(row: Row): string {
  if (row.origin !== 'inline') return row.reviewState ? `review (${row.reviewState.toLowerCase().replace('_', ' ')})` : 'PR comment'
  return row.line ? `${row.path}:${row.line}` : (row.path ?? '')
}

/** The table's location column: the file name and line, or "review" for a summary. */
function short(row: Row): string {
  if (row.origin === 'review-summary') return 'review'
  if (row.origin === 'issue-comment') return 'comment'
  const file = row.path?.split('/').pop() ?? ''
  return row.line ? `${file}:${row.line}` : file
}

/** The first line worth reading: skips blank lines and bare markdown headings. */
function firstLine(text: string): string {
  const lines = text.split('\n').map(s => s.trim()).filter(Boolean)
  const plain = lines.find(line => !/^#{1,6}\s/.test(line))
  return (plain ?? lines[0] ?? '').replace(/^#{1,6}\s+/, '')
}

function clip(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max)}…` : text
}
