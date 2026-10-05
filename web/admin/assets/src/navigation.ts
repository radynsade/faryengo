import htmx from 'htmx.org';

// History restoration needs full documents; ordinary HTMX requests use fragments.
htmx.config.historyRestoreAsHxRequest = false;
htmx.config.allowEval = false;
htmx.config.allowScriptTags = false;

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
		.querySelector<HTMLElement>('#panel-main, #auth-title')
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

document.addEventListener('htmx:afterSettle', focusPageContent);
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
