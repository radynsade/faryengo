import htmx from 'htmx.org';

// The trigger carries the entity's complete, localized dialog title, rendered
// by the server, so the script never builds user-facing text.
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
			dialog.querySelector<HTMLElement>('[data-confirm-title]')!.textContent =
				trigger.dataset.confirmTitle ?? '';
			error.hidden = true;
			error.textContent = '';
			htmx.process(form);
			dialog.showModal();
		}
	}
});
