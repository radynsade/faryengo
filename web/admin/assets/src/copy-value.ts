async function copyText(text: string): Promise<boolean> {
	if (window.isSecureContext && navigator.clipboard?.writeText) {
		try {
			await navigator.clipboard.writeText(text);
			return true;
		} catch {
			// Try the selection-based fallback if clipboard access is denied.
		}
	}

	const field = document.createElement('textarea');
	const focused = document.activeElement;
	const selection = window.getSelection();
	const ranges = Array.from(
		{ length: selection?.rangeCount ?? 0 },
		(_, index) => selection!.getRangeAt(index).cloneRange(),
	);
	field.value = text;
	field.readOnly = true;
	field.style.position = 'fixed';
	field.style.opacity = '0';
	field.style.pointerEvents = 'none';
	document.body.appendChild(field);

	try {
		field.focus({ preventScroll: true });
		field.select();
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		field.remove();
		if (focused instanceof HTMLElement) focused.focus({ preventScroll: true });
		if (selection) {
			selection.removeAllRanges();
			for (const range of ranges) selection.addRange(range);
		}
	}
}

function copyButton(target: EventTarget | null) {
	return target instanceof Element
		? target.closest<HTMLButtonElement>('button[data-copy-text]')
		: null;
}

const pending = new WeakSet<HTMLButtonElement>();

// Delegation also handles role details inserted by HTMX navigation.
document.addEventListener('click', async (event) => {
	const button = copyButton(event.target);
	const text = button?.dataset.copyText;
	if (button && text !== undefined && !pending.has(button)) {
		pending.add(button);
		const status = button.querySelector<HTMLElement>('[data-copy-status]');
		if (status) status.textContent = '';
		delete button.dataset.copyState;

		const copied = await copyText(text);
		pending.delete(button);
		if (button.isConnected) {
			if (copied) button.dataset.copyState = 'copied';
			if (status) {
				status.textContent =
					(copied ? button.dataset.copySuccess : button.dataset.copyError)
					?? '';
			}
		}
	}
});

function resetFeedback(event: MouseEvent | FocusEvent) {
	const button = copyButton(event.target);
	if (
		button
		&& !(
			event.relatedTarget instanceof Node
			&& button.contains(event.relatedTarget)
		)
		&& !pending.has(button)
	) {
		delete button.dataset.copyState;
		const status = button.querySelector<HTMLElement>('[data-copy-status]');
		if (status) status.textContent = '';
	}
}

document.addEventListener('mouseout', resetFeedback);
document.addEventListener('focusout', resetFeedback);
