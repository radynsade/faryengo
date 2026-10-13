// Opens a select's styled list from its toggle and keeps the native select in
// sync with the chosen option, announcing the change as the native select
// would, so auto-submitting selects save. The list closes on a click outside
// it, on Escape or Tab, and when a navigation starts. Delegated, so selects
// in content swapped in later work without setup.
type Parts = {
	root: HTMLElement;
	native: HTMLSelectElement;
	toggle: HTMLButtonElement;
	list: HTMLElement;
	options: HTMLElement[];
};

function parts(element: Element | null): Parts | null {
	const root = element?.closest<HTMLElement>('[data-select]');
	const native = root?.querySelector('select');
	const toggle = root?.querySelector<HTMLButtonElement>('.select__toggle');
	const list = root?.querySelector<HTMLElement>('[role="listbox"]');

	if (!root || !native || !toggle || !list) return null;

	return {
		root,
		native,
		toggle,
		list,
		options: [...list.querySelectorAll<HTMLElement>('[role="option"]')],
	};
}

// The list opens above its toggle when it would not fit below but fits above.
function open(select: Parts) {
	select.list.hidden = false;
	select.list.classList.remove('select__list--above');

	const list = select.list.getBoundingClientRect();
	const toggle = select.toggle.getBoundingClientRect();

	if (list.bottom > window.innerHeight && list.height < toggle.top) {
		select.list.classList.add('select__list--above');
	}

	select.toggle.setAttribute('aria-expanded', 'true');
	(
		select.options.find((option) => option.ariaSelected === 'true')
		?? select.options[0]
	)?.focus();
}

function close(select: Parts, focusToggle: boolean) {
	select.list.hidden = true;
	select.toggle.setAttribute('aria-expanded', 'false');

	if (focusToggle) select.toggle.focus();
}

function choose(select: Parts, chosen: HTMLElement) {
	const value = chosen.dataset.value ?? '';

	for (const option of select.options) {
		option.ariaSelected = String(option === chosen);
	}

	const label = select.toggle.querySelector('.select__value');

	if (label) label.textContent = chosen.textContent?.trim() ?? '';

	close(select, true);

	if (select.native.value !== value) {
		select.native.value = value;
		select.native.dispatchEvent(new Event('input', { bubbles: true }));
		select.native.dispatchEvent(new Event('change', { bubbles: true }));
	}
}

function openSelects() {
	return [
		...document.querySelectorAll<HTMLElement>(
			'[data-select] [role="listbox"]:not([hidden])',
		),
	]
		.map(parts)
		.filter((select) => select !== null);
}

document.addEventListener('click', (event) => {
	const target = event.target instanceof Element ? event.target : null;

	for (const select of openSelects()) {
		if (!target || !select.root.contains(target)) close(select, false);
	}

	const select = parts(target);
	const option = target?.closest<HTMLElement>('[role="option"]');

	if (select && option) {
		choose(select, option);
	} else if (select && target?.closest('.select__toggle')) {
		if (select.list.hidden) open(select);
		else close(select, true);
	} else if (target instanceof HTMLLabelElement) {
		// The label points at the hidden native select.
		parts(target.control)?.toggle.focus();
	}
});

document.addEventListener('keydown', (event) => {
	const target = event.target instanceof Element ? event.target : null;
	const select = parts(target);

	if (!select) return;

	if (target?.closest('.select__toggle')) {
		if (['ArrowDown', 'ArrowUp'].includes(event.key)) {
			event.preventDefault();
			open(select);
		}

		return;
	}

	const option = target?.closest<HTMLElement>('[role="option"]');

	if (!option) return;

	const index = select.options.indexOf(option);
	const last = select.options.length - 1;
	const moves: Record<string, number> = {
		ArrowDown: Math.min(index + 1, last),
		ArrowUp: Math.max(index - 1, 0),
		Home: 0,
		End: last,
	};

	if (event.key in moves) {
		event.preventDefault();
		select.options[moves[event.key]]?.focus();
	} else if (event.key === 'Enter' || event.key === ' ') {
		event.preventDefault();
		choose(select, option);
	} else if (event.key === 'Escape') {
		event.preventDefault();
		close(select, true);
	} else if (event.key === 'Tab') {
		close(select, false);
	}
});

document.addEventListener('htmx:beforeRequest', () => {
	for (const select of openSelects()) close(select, false);
});
