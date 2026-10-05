import { Idiomorph } from 'idiomorph/htmx';

// The server renders the complete initial layout; hydration only attaches listeners.
const instances = new WeakMap<HTMLElement, MultiSelect>();
const openWidgets = new Set<MultiSelect>();

function searchText(value: string) {
	return value
		.normalize('NFD')
		.replace(/\p{M}+/gu, '')
		.toLowerCase();
}

class MultiSelect {
	readonly root: HTMLElement;
	readonly control: HTMLElement;
	readonly select: HTMLSelectElement;
	private readonly input: HTMLInputElement;
	private readonly dropdown: HTMLElement;
	private readonly list: HTMLElement;
	private readonly values: HTMLElement;
	private readonly count: HTMLElement;
	private readonly status: HTMLElement;
	private readonly empty: HTMLElement;
	private readonly placeholder: string;
	private readonly selectedStatus: string[];
	private readonly options: HTMLOptionElement[];
	private readonly rows: HTMLElement[];
	private readonly events = new AbortController();
	private active = -1;

	constructor(root: HTMLElement) {
		this.root = root;
		this.control = root.querySelector('.multiselect__control')!;
		this.select = root.querySelector('select')!;
		this.input = root.querySelector('input')!;
		this.dropdown = root.querySelector('.multiselect__dropdown')!;
		this.list = root.querySelector('.multiselect__options')!;
		this.values = root.querySelector('.multiselect__values')!;
		this.count = root.querySelector('.multiselect__count')!;
		this.status = root.querySelector('.multiselect__status')!;
		this.empty = root.querySelector('.multiselect__empty')!;
		this.placeholder = this.input.dataset.placeholder ?? '';
		this.selectedStatus = JSON.parse(root.dataset.selectedStatus!);
		this.options = Array.from(this.select.options);
		this.rows = Array.from(
			this.list.querySelectorAll<HTMLElement>('[role="option"]'),
		);
		const signal = this.events.signal;
		const label = root.querySelector<HTMLLabelElement>('.multiselect__label')!;

		label.addEventListener(
			'click',
			(event) => {
				// Labels normally forward a click to their input; only move focus here.
				event.preventDefault();
				this.close();
				this.input.focus({ preventScroll: true });
			},
			{ signal },
		);

		// Restoring focus after removing a tag must not open the dropdown.
		this.input.addEventListener('click', () => this.open(), { signal });
		this.input.addEventListener('input', () => this.open(), { signal });
		this.input.addEventListener('keydown', (event) => this.keydown(event), {
			signal,
		});
		this.list.addEventListener(
			'pointerdown',
			(event) => {
				// Keep keyboard focus on the combobox while toggling options.
				event.preventDefault();
			},
			{ signal },
		);
		this.list.addEventListener(
			'click',
			(event) => {
				const row =
					event.target instanceof Element
						? event.target.closest<HTMLElement>('[data-index]')
						: null;
				if (row) this.toggle(Number(row.dataset.index));
			},
			{ signal },
		);
		this.values.addEventListener(
			'click',
			(event) => {
				const badge =
					event.target instanceof Element
						? event.target.closest<HTMLButtonElement>('button[data-index]')
						: null;
				if (badge && !badge.disabled) {
					this.close();
					this.toggle(Number(badge.dataset.index));
					this.input.focus({ preventScroll: true });
				}
			},
			{ signal },
		);
		this.select.addEventListener('change', () => this.render(), { signal });
		this.select.form?.addEventListener(
			'reset',
			(event) => {
				// Reset runs before the browser restores the native values.
				queueMicrotask(() => {
					if (!event.defaultPrevented) {
						this.close();
						this.render();
					}
				});
			},
			{ signal },
		);
	}

	private render() {
		const selected = this.options.filter((option) => option.selected);
		this.input.placeholder = selected.length
			? selected.map((option) => option.label).join(', ')
			: this.placeholder;
		this.count.textContent = String(selected.length);
		this.count.hidden = !selected.length;
		this.status.textContent = this.selectedStatus[selected.length]!;
		this.values.replaceChildren();
		this.options.forEach((option, index) => {
			const row = this.rows[index]!;
			row.setAttribute('aria-selected', String(option.selected));
			row.setAttribute('aria-disabled', String(option.disabled));
			if (option.selected) {
				const badge = document.createElement('button');
				badge.type = 'button';
				badge.className = 'multiselect__value';
				badge.dataset.index = String(index);
				badge.disabled = option.disabled || this.select.disabled;
				badge.setAttribute('aria-label', row.dataset.removeLabel!);
				badge.append(document.createTextNode(option.label));
				const icon = document.createElement('i');
				icon.className = 'ti ti-x';
				icon.setAttribute('aria-hidden', 'true');
				badge.append(icon);
				this.values.append(badge);
			}
		});
		this.filter();
	}

	private filter() {
		const query = searchText(this.input.value);
		this.rows.forEach((row, index) => {
			row.hidden = !searchText(this.options[index]!.label).includes(query);
		});
		this.empty.hidden = this.rows.some((row) => !row.hidden);
		this.activate(-1);
	}

	private activate(index: number) {
		this.rows.forEach((row, rowIndex) => {
			row.classList.toggle('multiselect__option--active', rowIndex === index);
		});
		this.active = index;
		if (index < 0) this.input.removeAttribute('aria-activedescendant');
		else {
			this.input.setAttribute('aria-activedescendant', this.rows[index]!.id);
			this.rows[index]!.scrollIntoView({ block: 'nearest' });
		}
	}

	private toggle(index: number) {
		const option = this.options[index];
		if (!option || option.disabled || this.select.disabled) return;
		option.selected = !option.selected;
		this.input.value = '';
		this.select.dispatchEvent(new Event('change', { bubbles: true }));
	}

	private open() {
		if (this.select.disabled) return;
		for (const widget of openWidgets) {
			if (widget !== this) widget.close();
		}
		this.filter();
		this.dropdown.hidden = false;
		this.input.setAttribute('aria-expanded', 'true');
		openWidgets.add(this);
	}

	close() {
		this.dropdown.hidden = true;
		this.input.setAttribute('aria-expanded', 'false');
		this.input.value = '';
		this.activate(-1);
		openWidgets.delete(this);
	}

	destroy() {
		this.close();
		this.events.abort();
		instances.delete(this.root);
	}

	private keydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			if (!this.dropdown.hidden) {
				event.preventDefault();
				event.stopPropagation();
				this.close();
			}
		} else if (event.key === 'Tab') this.close();
		else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
			event.preventDefault();
			if (this.dropdown.hidden) this.open();
			const visible = this.rows.flatMap((row, index) =>
				!row.hidden && !this.options[index]!.disabled ? [index] : [],
			);
			const direction = event.key === 'ArrowDown' ? 1 : -1;
			const position = visible.indexOf(this.active);
			const next =
				position < 0
					? direction > 0
						? 0
						: visible.length - 1
					: (position + direction + visible.length) % visible.length;
			this.activate(visible[next] ?? -1);
		} else if (event.key === 'Enter') {
			event.preventDefault();
			if (this.dropdown.hidden) this.open();
			else {
				const visible = this.rows.flatMap((row, index) =>
					!row.hidden && !this.options[index]!.disabled ? [index] : [],
				);
				if (this.active >= 0) this.toggle(this.active);
				else if (visible.length === 1) this.toggle(visible[0]!);
			}
		}
	}
}

function rootsWithin(element: Element) {
	const roots = Array.from(
		element.querySelectorAll<HTMLElement>('[data-multiselect]'),
	);
	if (element instanceof HTMLElement && element.matches('[data-multiselect]')) {
		roots.unshift(element);
	}
	return roots;
}

function initialize() {
	for (const root of rootsWithin(document.documentElement)) {
		if (!instances.has(root)) instances.set(root, new MultiSelect(root));
	}
}

// Delegated document listeners persist across swaps without retaining removed fields.
for (const type of ['pointerdown', 'focusin']) {
	document.addEventListener(type, (event) => {
		for (const widget of openWidgets) {
			if (
				event.target instanceof Node
				&& !widget.control.contains(event.target)
			) {
				widget.close();
			}
		}
	});
}

function cleanup(element: Node) {
	if (element instanceof Element) {
		for (const root of rootsWithin(element)) instances.get(root)?.destroy();
	}
	return true;
}

document.addEventListener('htmx:beforeCleanupElement', (event) => {
	cleanup((event as CustomEvent<{ elt: Element }>).detail.elt);
});

// Morphs can retain a widget root while replacing its controls and option rows.
// Rehydrate from the morphed server markup on htmx:load without stale references.
Idiomorph.defaults.callbacks.beforeNodeMorphed = (node) => {
	if (node instanceof HTMLElement) instances.get(node)?.destroy();
	return true;
};
Idiomorph.defaults.callbacks.beforeNodeRemoved = cleanup;

document.addEventListener('htmx:load', initialize);
document.addEventListener('htmx:historyRestore', initialize);
initialize();
