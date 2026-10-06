// Delegation also handles filter panels rendered by HTMX swaps.
document.addEventListener('click', (event) => {
	if (event.target instanceof Node) {
		// Keep the original ancestry even if a badge removes itself during the click.
		const path = event.composedPath();

		for (const panel of document.querySelectorAll<HTMLDetailsElement>(
			'details.role-filters[open]',
		)) {
			if (!path.includes(panel)) {
				panel.open = false;
			}
		}
	}
});

// Role-loading and filter errors display the server's localized feedback.
document.addEventListener('htmx:beforeSwap', (event) => {
	const detail = (event as CustomEvent).detail;

	if (
		detail.target instanceof HTMLElement
		&& ['roles-list-table', 'page-content'].includes(detail.target.id)
		&& detail.xhr.getResponseHeader('HX-Retarget') === `#${detail.target.id}`
	) {
		detail.shouldSwap = true;
		detail.isError = false;
	}
});
