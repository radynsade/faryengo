import { action, actions } from './vendor/datastar.js'

let pendingNavigation: AbortController | undefined

// Keep Datastar's request and DOM patching behavior, adding browser history
// and cancellation across different page URLs.
action({
  name: 'navigate',
  async apply(context, href, pushHistory = true) {
    const event = context.evt

    if (event instanceof MouseEvent) {
      if (event.defaultPrevented || event.button !== 0 || event.ctrlKey ||
          event.metaKey || event.shiftKey || event.altKey) {
        return
      }
    }

    const url = new URL(String(href), window.location.href)

    if (url.origin !== window.location.origin) {
      return
    }

    event?.preventDefault()
    pendingNavigation?.abort()

    const controller = new AbortController()
    pendingNavigation = controller
    const errorMessage = document.getElementById('navigation-error')
    if (errorMessage) errorMessage.hidden = true

    let patched = false
    const onFetch = (event: Event) => {
      const detail = (event as CustomEvent<{ type: string; el: Element }>).detail
      if (detail.el === context.el && detail.type === 'datastar-patch-elements') {
        patched = true
      }
    }

    document.addEventListener('datastar-fetch', onFetch)

    try {
      await actions.get(context, url.href, {
        filterSignals: { include: /^$/ },
        requestCancellation: controller,
        openWhenHidden: true,
        retry: 'never',
      })

      if (!controller.signal.aborted) {
        if (!patched) throw new Error('Page content was not received')

        if (pushHistory && url.href !== window.location.href) {
          history.pushState(null, '', url.href)
        }

        document.getElementById('auth-title')?.focus({ preventScroll: true })
      }
    } catch {
      if (!controller.signal.aborted && errorMessage) {
        errorMessage.hidden = false
      }
    } finally {
      document.removeEventListener('datastar-fetch', onFetch)
      if (pendingNavigation === controller) pendingNavigation = undefined
    }
  },
})
