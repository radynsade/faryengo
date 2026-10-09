// Opens and closes the dashboard's drawer on narrow screens. Delegated, so it
// keeps working when the page content is morphed.
function setOpen(dashboard: HTMLElement, open: boolean, moveFocus = true) {
	dashboard.classList.toggle('dashboard--menu-open', open);
	dashboard
		.querySelector('[data-sidebar-open]')
		?.setAttribute('aria-expanded', String(open));

	if (!moveFocus) {
		// Navigation moves focus to the new content itself.
	} else if (open) {
		dashboard.querySelector<HTMLElement>('.sidebar__close')?.focus();
	} else {
		dashboard.querySelector<HTMLElement>('[data-sidebar-open]')?.focus();
	}
}

function openDashboard() {
	return document.querySelector<HTMLElement>('.dashboard--menu-open');
}

document.addEventListener('click', (event) => {
	const target = event.target instanceof Element ? event.target : null;
	const dashboard = target?.closest<HTMLElement>('[data-dashboard]');

	if (dashboard && target?.closest('[data-sidebar-open]')) {
		setOpen(dashboard, true);
	} else if (dashboard && target?.closest('[data-sidebar-close]')) {
		setOpen(dashboard, false);
	}
});

document.addEventListener('keydown', (event) => {
	const dashboard = openDashboard();

	if (event.key === 'Escape' && dashboard) setOpen(dashboard, false);
});

// Choosing a page in the drawer closes it as the navigation starts.
document.addEventListener('htmx:beforeRequest', () => {
	const dashboard = openDashboard();

	if (dashboard) setOpen(dashboard, false, false);
});
