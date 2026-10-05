import htmx from 'htmx.org';

document.addEventListener('click', (event) => {
	const trigger =
		event.target instanceof Element
			? event.target.closest<HTMLButtonElement>('[data-confirm-delete]')
			: null;

	if (trigger) {
		const dialog = document.getElementById(trigger.dataset.confirmDelete ?? '');

		if (dialog instanceof HTMLDialogElement) {
			const form = dialog.querySelector<HTMLFormElement>('form')!;
			const error = dialog.querySelector<HTMLElement>('[data-confirm-error]')!;
			form.action = trigger.dataset.confirmAction!;
			form.setAttribute('hx-post', trigger.dataset.confirmAction!);
			dialog.querySelector<HTMLElement>('[data-confirm-name]')!.textContent =
				trigger.dataset.confirmName ?? '';
			error.hidden = true;
			error.textContent = '';
			htmx.process(form);
			dialog.showModal();
		}
	}
});

// Failed deletions replace only the dialog, keeping the underlying page in place.
document.addEventListener('htmx:beforeSwap', (event) => {
	const detail = (event as CustomEvent).detail;

	if (
		detail.target instanceof HTMLDialogElement
		&& detail.xhr.getResponseHeader('HX-Retarget') === `#${detail.target.id}`
	) {
		detail.shouldSwap = true;
		detail.isError = false;
	}
});
