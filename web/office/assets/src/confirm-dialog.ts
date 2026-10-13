// Asks for the current password in a modal before a form submits a change
// that needs it, and turns a confirmation the server opened into a modal.
// Delegated, so it keeps working when the forms are morphed.

// A form needs confirmation always, or when the named field differs from the
// value it was rendered with. A repeated field that does not match yet, such as
// a new password, sends the form without it, so the server reports the
// mismatch before asking for the current password.
function repeatsMatch(form: HTMLFormElement) {
	return [
		...form.querySelectorAll<HTMLInputElement>('[data-must-match]'),
	].every((repeat) => {
		const original = form.elements.namedItem(repeat.dataset.mustMatch!);

		return (
			original instanceof HTMLInputElement && original.value === repeat.value
		);
	});
}

function needsConfirmation(form: HTMLFormElement) {
	const rule = form.dataset.confirmPassword;
	const field = rule ? form.elements.namedItem(rule) : null;

	return (
		repeatsMatch(form)
		&& (rule === 'always'
			|| (field instanceof HTMLInputElement
				&& field.value !== field.defaultValue))
	);
}

function showModal(dialog: HTMLDialogElement) {
	if (!dialog.matches(':modal')) {
		if (dialog.open) dialog.close();
		dialog.showModal();
	}
}

function upgradeOpenDialogs() {
	for (const dialog of document.querySelectorAll<HTMLDialogElement>(
		'dialog[data-confirm-dialog][open]',
	)) {
		showModal(dialog);
	}
}

// htmx asks before every request; a submission from the form's own button
// that needs confirmation waits for the dialog's Confirm instead.
document.addEventListener('htmx:confirm', (event) => {
	const form = event.target instanceof HTMLFormElement ? event.target : null;
	const trigger = (event as CustomEvent).detail?.triggeringEvent;
	const submitter = trigger instanceof SubmitEvent ? trigger.submitter : null;
	const dialog = form?.querySelector<HTMLDialogElement>(
		'dialog[data-confirm-dialog]',
	);

	if (
		form
		&& dialog
		&& !submitter?.hasAttribute('data-confirm-submit')
		&& needsConfirmation(form)
	) {
		event.preventDefault();
		showModal(dialog);
	}
});

// Cancel closes the modal.
document.addEventListener('click', (event) => {
	const target = event.target instanceof Element ? event.target : null;
	const dialog = target?.closest('[data-dialog-close]')?.closest('dialog');

	if (dialog) dialog.close();
});

// A click on the backdrop closes the modal. The backdrop belongs to the
// dialog element, so the click lands on the dialog itself, outside its box.
// The press must begin there too, so a selection dragged out of the dialog
// does not close it.
function outsideModal(event: MouseEvent) {
	const dialog =
		event.target instanceof HTMLDialogElement ? event.target : null;
	const box = dialog?.getBoundingClientRect();

	return dialog?.matches('[data-confirm-dialog]:modal')
		&& box
		&& (event.clientX < box.left
			|| event.clientX > box.right
			|| event.clientY < box.top
			|| event.clientY > box.bottom)
		? dialog
		: null;
}

let pressedOutside: HTMLDialogElement | null = null;

document.addEventListener('pointerdown', (event) => {
	pressedOutside = outsideModal(event);
});

document.addEventListener('click', (event) => {
	const dialog = outsideModal(event);

	if (dialog && dialog === pressedOutside) dialog.close();

	pressedOutside = null;
});

// A response that replaces anything but the confirmation itself ends it.
document.addEventListener('htmx:beforeSwap', (event) => {
	const target = (event as CustomEvent).detail?.target;

	for (const dialog of document.querySelectorAll<HTMLDialogElement>(
		'dialog[data-confirm-dialog][open]',
	)) {
		if (dialog !== target) dialog.close();
	}
});

document.addEventListener('htmx:afterSettle', upgradeOpenDialogs);
document.addEventListener('DOMContentLoaded', upgradeOpenDialogs);
