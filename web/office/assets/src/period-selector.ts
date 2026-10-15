// Keeps a period selector's fields in step with its window and repeat, fills
// it from a preset, and sends the device's time zone with a period that has
// none yet. The server renders the same fields shown or hidden, so this only
// follows changes. Delegated, so selectors in content swapped in later work
// without setup.
const selectorRoot = '[data-period-selector]';

function selectIn(root: Element, name: string) {
	return root.querySelector<HTMLSelectElement>(`select[name="${name}"]`);
}

function sync(root: Element) {
	const repeat = selectIn(root, 'period_repeat')?.value ?? '';
	const window = selectIn(root, 'period_window')?.value ?? 'custom';

	for (const element of root.querySelectorAll<HTMLElement>(
		'[data-period-repeating]',
	)) {
		element.hidden = repeat === '';
	}

	for (const element of root.querySelectorAll<HTMLElement>(
		'[data-period-open]',
	)) {
		element.hidden = repeat !== '' || window !== 'custom';
	}

	for (const element of root.querySelectorAll<HTMLElement>(
		'[data-period-every]',
	)) {
		element.hidden = repeat !== 'every';
	}

	for (const element of root.querySelectorAll<HTMLElement>(
		'[data-period-window-show]',
	)) {
		element.hidden = !(element.dataset.periodWindowShow ?? '')
			.split(' ')
			.includes(window);
	}
}

// A whole window allows only the repeats the server lists for it, and the
// first of them replaces a repeat that does not fit; a custom range allows
// every repeat.
function fitRepeat(root: HTMLElement) {
	const window = selectIn(root, 'period_window')?.value ?? 'custom';
	const repeat = selectIn(root, 'period_repeat');
	const fits: Record<string, string[]> = JSON.parse(
		root.dataset.periodFits ?? '{}',
	);
	const allowed = fits[window];

	for (const option of repeat?.options ?? []) {
		option.disabled = allowed !== undefined && !allowed.includes(option.value);
	}

	for (const option of repeat
		?.closest('[data-select]')
		?.querySelectorAll<HTMLElement>('[role="option"]') ?? []) {
		if (allowed === undefined || allowed.includes(option.dataset.value ?? '')) {
			option.removeAttribute('aria-disabled');
		} else {
			option.setAttribute('aria-disabled', 'true');
		}
	}

	if (repeat && allowed && !allowed.includes(repeat.value)) {
		choose(root, 'period_repeat', allowed[0] ?? '');
	}
}

const pad = (value: number) => String(value).padStart(2, '0');

// A date-time input takes the local time without its zone.
function localValue(date: Date) {
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function dateValue(date: Date) {
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

// A select is changed through its styled list, so the list, its label, and
// the native select stay in step and the change is announced.
function choose(root: Element, name: string, value: string) {
	const select = selectIn(root, name);
	const option = select
		?.closest('[data-select]')
		?.querySelector<HTMLElement>(`[role="option"][data-value="${value}"]`);

	if (select && select.value !== value) option?.click();
}

function setInput(root: Element, name: string, value: string) {
	const input = root.querySelector<HTMLInputElement>(`input[name="${name}"]`);

	if (input) input.value = value;
}

// Each preset starts with the current day, week (from Monday), or month, in
// the device's calendar. A week is a custom range; the others are whole.
function applyPreset(root: Element, preset: string) {
	const now = new Date();
	const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());

	setInput(root, 'period_day', dateValue(today));
	setInput(root, 'period_year', String(today.getFullYear()));
	choose(root, 'period_month', String(today.getMonth() + 1));

	switch (preset) {
		case 'daily':
			choose(root, 'period_window', 'day');
			choose(root, 'period_repeat', 'daily');
			break;
		case 'weekly': {
			const monday = new Date(
				today.getFullYear(),
				today.getMonth(),
				today.getDate() - ((today.getDay() + 6) % 7),
			);

			setInput(root, 'period_from', localValue(monday));
			setInput(
				root,
				'period_to',
				localValue(
					new Date(
						monday.getFullYear(),
						monday.getMonth(),
						monday.getDate() + 7,
					),
				),
			);
			choose(root, 'period_window', 'custom');
			choose(root, 'period_repeat', 'weekly');
			break;
		}
		case 'monthly':
			choose(root, 'period_window', 'month');
			choose(root, 'period_repeat', 'monthly');
			break;
		default:
			choose(root, 'period_window', 'month');
			choose(root, 'period_repeat', '');
	}

	sync(root);
}

document.addEventListener('change', (event) => {
	if (
		event.target instanceof HTMLSelectElement
		&& ['period_repeat', 'period_window'].includes(event.target.name)
	) {
		const root = event.target.closest<HTMLElement>(selectorRoot);

		if (root && event.target.name === 'period_window') fitRepeat(root);
		if (root) sync(root);
	}
});

document.addEventListener('click', (event) => {
	const button =
		event.target instanceof Element
			? event.target.closest<HTMLElement>('[data-period-preset]')
			: null;
	const root = button?.closest(selectorRoot);

	if (button && root) applyPreset(root, button.dataset.periodPreset ?? '');
});

// Every way of sending a form builds its data from the form, so the zone is
// added there; the field keeps the zone an existing period already has.
document.addEventListener('formdata', (event) => {
	const form = event.target instanceof HTMLFormElement ? event.target : null;

	if (
		form?.querySelector('[data-period-time-zone]')
		&& !event.formData.get('time_zone')
	) {
		event.formData.set(
			'time_zone',
			Intl.DateTimeFormat().resolvedOptions().timeZone,
		);
	}
});
