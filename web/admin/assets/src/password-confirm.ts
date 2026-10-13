// Asks for the current password in a modal before a form submits a change
// that needs it. Delegated, so it keeps working when the forms are morphed.
// Opening a dialog the server asks for, closing on the backdrop, and the
// close control, including Cancel, belong to the shared dialog behavior.

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
		dialog.showModal();
	}
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
