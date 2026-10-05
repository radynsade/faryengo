document.addEventListener('click', (event) => {
	const close =
		event.target instanceof Element
			? event.target.closest('[data-dialog-close]')
			: null;
	const dialog = close?.closest('dialog[data-dialog]');

	if (dialog instanceof HTMLDialogElement) {
		dialog.close();
	}

	if (
		event.target instanceof HTMLDialogElement
		&& event.target.matches('[data-dialog]')
	) {
		const bounds = event.target.getBoundingClientRect();

		if (
			event.clientX < bounds.left
			|| event.clientX > bounds.right
			|| event.clientY < bounds.top
			|| event.clientY > bounds.bottom
		) {
			event.target.close();
		}
	}
});

function initialize() {
	for (const dialog of document.querySelectorAll<HTMLDialogElement>(
		'dialog[data-dialog-open]',
	)) {
		if (!dialog.open) {
			dialog.showModal();
		}

		dialog.removeAttribute('data-dialog-open');
	}
}

document.addEventListener('htmx:load', initialize);
initialize();
