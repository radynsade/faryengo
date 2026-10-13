// A select marked for it submits its form as soon as it changes, through the
// form's own submission, so in-page handling applies. Delegated, so selects
// in content swapped in later work without setup.
document.addEventListener('change', (event) => {
	const select =
		event.target instanceof HTMLSelectElement
		&& event.target.hasAttribute('data-auto-submit')
			? event.target
			: null;

	select?.form?.requestSubmit();
});
