// Delegated, so toggles in content swapped in later work without setup.
document.addEventListener('click', (event) => {
	const toggle =
		event.target instanceof Element
			? event.target.closest<HTMLButtonElement>('[data-password-toggle]')
			: null;
	const input = toggle
		? document.getElementById(toggle.getAttribute('aria-controls') ?? '')
		: null;

	if (toggle && input instanceof HTMLInputElement) {
		const visible = input.type === 'password';

		input.type = visible ? 'text' : 'password';
		toggle.setAttribute('aria-pressed', String(visible));
		toggle.setAttribute(
			'aria-label',
			(visible ? toggle.dataset.labelHide : toggle.dataset.labelShow) ?? '',
		);
	}
});
