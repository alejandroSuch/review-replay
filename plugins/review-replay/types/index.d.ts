export type Status = 'addressed' | 'partial' | 'pending' | 'needs-discussion'
export type Confidence = 'high' | 'medium' | 'low'

/** One reviewer comment and, once classified, its verdict. */
export type Row = {
  commentId: number
  origin: 'inline' | 'review-summary' | 'issue-comment'
  author: string
  path?: string
  line?: number
  body: string
  htmlUrl: string
  reviewState?: string
  /** Unset while the model is still working on it. */
  status?: Status
  confidence?: Confidence
  rationale?: string
  draftReply?: string
  /** `rule:<name>` for a short-circuit, `model` for a classification, `error` when the call failed. */
  source?: string
}

export type Phase = 'idle' | 'fetching' | 'classifying' | 'done' | 'error'

/** The PR the pane shows. */
export type Run = {
  phase: Phase
  pr: string
  url: string
  prAuthor: string
  commits: number
  historyRewritten: boolean
  error?: string
  rows: Row[]
}

declare module 'claude-code' {
  interface PluginState {
    'review-replay': { run: Run; selected: number }
  }
}
