// Closes an open dropdown on a click outside it, on Escape, which returns
// focus to its toggle, and when a navigation starts. Delegated, so it keeps
// working when the top bar is morphed.
function openDropdowns() {
	return document.querySelectorAll<HTMLDetailsElement>(
		'details[data-dropdown][open]',
	);
}

document.addEventListener('click', (event) => {
	const target = event.target instanceof Node ? event.target : null;

	for (const dropdown of openDropdowns()) {
		if (!target || !dropdown.contains(target)) dropdown.open = false;
	}
});

document.addEventListener('keydown', (event) => {
	for (const dropdown of event.key === 'Escape' ? openDropdowns() : []) {
		dropdown.open = false;
		dropdown.querySelector<HTMLElement>('summary')?.focus();
	}
});

document.addEventListener('htmx:beforeRequest', () => {
	for (const dropdown of openDropdowns()) dropdown.open = false;
});
