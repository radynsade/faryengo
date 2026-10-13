// A message that has faded away leaves the page, so the messages below it
// move up and a later morph never meets it. Delegated, so it covers messages
// that arrive with a morph.
document.addEventListener('animationend', (event) => {
	if (
		event.target instanceof HTMLElement
		&& event.target.matches('[data-message]')
		&& ['message-out', 'message-fade'].includes(event.animationName)
	) {
		event.target.remove();
	}
});
