// Types for the Datastar v1.0.4 action APIs used by this application.
export type ActionContext = {
  el: HTMLElement | SVGElement | MathMLElement
  evt?: Event
  error: (name: string, context?: Record<string, unknown>) => Error
  cleanups: Map<string, () => void>
}

export declare function action<T>(plugin: {
  name: string
  apply: (context: ActionContext, ...args: unknown[]) => T
}): void

export declare const actions: {
  get(context: ActionContext, url: string, options?: {
    filterSignals?: { include: RegExp }
    requestCancellation?: AbortController
    openWhenHidden?: boolean
    retry?: 'never'
  }): Promise<void>
}
