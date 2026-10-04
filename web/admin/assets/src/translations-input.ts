const initialized = new WeakSet<HTMLElement>();

function tabs(root: HTMLElement) {
	return Array.from(root.querySelectorAll<HTMLButtonElement>('[role="tab"]'));
}

function activate(root: HTMLElement, tab: HTMLButtonElement, focus = false) {
	for (const button of tabs(root)) {
		const selected = button === tab;
		button.setAttribute('aria-selected', String(selected));
		button.tabIndex = selected ? 0 : -1;
	}

	for (const panel of Array.from(
		root.querySelectorAll<HTMLElement>('.translations-input__panel'),
	)) {
		panel.hidden = panel.id !== tab.getAttribute('aria-controls');
	}

	if (focus) tab.focus({ preventScroll: true });
}

function initialTab(root: HTMLElement) {
	const buttons = tabs(root);
	return (
		buttons.find((tab) => tab.dataset.language === root.dataset.language)
		?? buttons[0]
	);
}

function initialize() {
	for (const root of Array.from(
		document.querySelectorAll<HTMLElement>('[data-translations-input]'),
	)) {
		if (!initialized.has(root)) {
			const tab = initialTab(root);
			if (tab) {
				for (const button of tabs(root)) {
					const panel = Array.from(
						root.querySelectorAll<HTMLElement>('.translations-input__panel'),
					).find((item) => item.id === button.getAttribute('aria-controls'));
					panel?.setAttribute('role', 'tabpanel');
					panel?.setAttribute('aria-labelledby', button.id);
				}
				activate(root, tab);
				root.querySelector<HTMLElement>('[role="tablist"]')!.hidden = false;
			}
			initialized.add(root);
		}
	}
}

// Delegation keeps tabs working after HTMX swaps without per-field document listeners.
document.addEventListener('click', (event) => {
	const tab =
		event.target instanceof Element
			? event.target.closest<HTMLButtonElement>('.translations-input__tab')
			: null;
	const root = tab?.closest<HTMLElement>('[data-translations-input]');
	if (tab && root) activate(root, tab);
});

document.addEventListener('keydown', (event) => {
	const tab =
		event.target instanceof Element
			? event.target.closest<HTMLButtonElement>('.translations-input__tab')
			: null;
	const root = tab?.closest<HTMLElement>('[data-translations-input]');
	if (
		tab
		&& root
		&& ['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)
	) {
		event.preventDefault();
		const buttons = tabs(root);
		let index = buttons.indexOf(tab);
		if (event.key === 'Home') index = 0;
		else if (event.key === 'End') index = buttons.length - 1;
		else
			index =
				(index + (event.key === 'ArrowRight' ? 1 : -1) + buttons.length)
				% buttons.length;
		activate(root, buttons[index]!, true);
	}
});

document.addEventListener('reset', (event) => {
	if (event.target instanceof HTMLFormElement) {
		const form = event.target;
		queueMicrotask(() => {
			if (!event.defaultPrevented) {
				for (const root of Array.from(
					form.querySelectorAll<HTMLElement>('[data-translations-input]'),
				)) {
					const tab = initialTab(root);
					if (tab) activate(root, tab);
				}
			}
		});
	}
});

// Reveal an invalid language field before the browser moves focus to it.
document.addEventListener(
	'invalid',
	(event) => {
		const panel =
			event.target instanceof Element
				? event.target.closest<HTMLElement>('.translations-input__panel')
				: null;
		const root = panel?.closest<HTMLElement>('[data-translations-input]');
		if (panel && root) {
			const tab = tabs(root).find(
				(button) => button.getAttribute('aria-controls') === panel.id,
			);
			if (tab) activate(root, tab);
		}
	},
	true,
);

document.addEventListener('htmx:load', initialize);
document.addEventListener('htmx:historyRestore', initialize);
initialize();
