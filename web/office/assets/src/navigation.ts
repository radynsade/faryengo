import htmx from 'htmx.org';
import 'htmx-ext-head-support';
import 'idiomorph/htmx';

// History restoration needs full documents; ordinary requests use head/content fragments.
htmx.config.historyRestoreAsHxRequest = false;
htmx.config.allowEval = false;
htmx.config.allowScriptTags = false;

// Navigation replaces either the whole page region or, inside the dashboard,
// only its main region; both are page changes for focus.
const pageRegions = ['page-content', 'dashboard-main'];

function showNavigationError(show: boolean) {
	const message = document.getElementById('navigation-error');

	if (message) message.hidden = !show;
}

function focusPageContent() {
	const content = document.querySelector<HTMLElement>('[data-ui-language]');

	if (content) {
		document.documentElement.lang = content.dataset.uiLanguage!;
		const error = document.getElementById('navigation-error');
		if (error) error.textContent = content.dataset.navigationError!;
	}

	showNavigationError(false);
	document
		.querySelector<HTMLElement>('#dashboard-main, #auth-title')
		?.focus({ preventScroll: true });
}

// Let the browser handle modified clicks before HTMX cancels the link event.
document.addEventListener(
	'click',
	(event) => {
		if (
			(event.defaultPrevented
				|| event.button !== 0
				|| event.ctrlKey
				|| event.metaKey
				|| event.shiftKey
				|| event.altKey)
			&& event.target instanceof Element
			&& event.target.closest('a[hx-boost="true"]')
		) {
			event.stopImmediatePropagation();
		}
	},
	true,
);

document.addEventListener('htmx:beforeRequest', () =>
	showNavigationError(false),
);

// The server answers failed validation and rejected sign-ins with rendered
// HTML, which replaces its target like any other result. Other error
// responses keep the current page and show the translated navigation error.
document.addEventListener('htmx:beforeSwap', (event) => {
	const detail = (event as CustomEvent).detail;
	const type = detail.xhr?.getResponseHeader('Content-Type') ?? '';

	if (detail.isError && type.startsWith('text/html')) {
		detail.shouldSwap = true;
		detail.isError = false;
	}

	if (
		detail.shouldSwap
		&& detail.target instanceof HTMLElement
		&& pageRegions.includes(detail.target.id)
	) {
		// htmx reports the swap on the element that sent the request. When
		// the new page replaced that element, such as the sign-out form, the
		// event fires on a detached node and never reaches the document, so
		// the head merge and focus handling would not run. The swap itself
		// runs synchronously after this event, so a task queued here sees
		// its result and relays the event from the body.
		const requester = detail.elt;

		setTimeout(() => {
			if (requester instanceof Element && !requester.isConnected) {
				document.body.dispatchEvent(
					new CustomEvent('htmx:afterSwap', { bubbles: true, detail }),
				);
			}
		});
	}
});

document.addEventListener('htmx:afterSwap', (event) => {
	const detail = (event as CustomEvent).detail;

	if (
		detail.target instanceof HTMLElement
		&& pageRegions.includes(detail.target.id)
	) {
		focusPageContent();
	}
});
document.addEventListener('htmx:historyRestore', focusPageContent);

for (const event of [
	'htmx:responseError',
	'htmx:sendError',
	'htmx:timeout',
	'htmx:swapError',
	'htmx:historyCacheMissLoadError',
]) {
	document.addEventListener(event, () => showNavigationError(true));
}
