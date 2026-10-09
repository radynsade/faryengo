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
