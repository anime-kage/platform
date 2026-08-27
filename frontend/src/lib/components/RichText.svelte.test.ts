import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import RichText from './RichText.svelte';

// RichText renders Spoiler internally, so these cover the whole
// comment / review / chat text path end to end.

describe('RichText — plain text', () => {
	it('renders the text', () => {
		const { container } = render(RichText, { props: { text: 'Un comentariu normal.' } });
		expect(container.textContent).toBe('Un comentariu normal.');
	});

	it('renders html as literal characters', () => {
		// Svelte escapes interpolated text; there is no {@html} on this path.
		const { container } = render(RichText, {
			props: { text: '<script>alert(1)</script>' }
		});
		expect(container.querySelector('script')).toBeNull();
		expect(container.textContent).toBe('<script>alert(1)</script>');
	});

	it('renders nothing for empty text', () => {
		const { container } = render(RichText, { props: { text: '' } });
		expect(container.textContent).toBe('');
	});
});

describe('RichText — spoilers', () => {
	it('hides spoiler text behind a button', () => {
		render(RichText, { props: { text: 'A murit ||personajul||.' } });
		const button = screen.getByRole('button', { name: /spoiler/i });
		expect(button).toBeInTheDocument();
		// The text is present so layout does not jump on reveal — it is
		// painted out, not removed.
		expect(button).toHaveTextContent('personajul');
	});

	it('reveals on click and does not offer to hide again', async () => {
		render(RichText, { props: { text: '||secret||' } });
		await fireEvent.click(screen.getByRole('button', { name: /spoiler/i }));
		expect(screen.queryByRole('button')).toBeNull();
		expect(screen.getByText('secret')).toBeInTheDocument();
	});

	it('renders no button when interactive is false', () => {
		// A button cannot nest inside an <a>, and these rows are links.
		render(RichText, { props: { text: '||secret||', interactive: false } });
		expect(screen.queryByRole('button')).toBeNull();
		expect(screen.getByLabelText(/conține spoiler/i)).toBeInTheDocument();
	});

	it('keeps the surrounding text visible', () => {
		const { container } = render(RichText, { props: { text: 'inainte ||ascuns|| dupa' } });
		expect(container.textContent).toContain('inainte');
		expect(container.textContent).toContain('dupa');
	});
});

describe('RichText — gifs', () => {
	const GIF = 'https://media.giphy.com/media/abc/giphy.gif';

	it('renders a bare giphy link as an image', () => {
		render(RichText, { props: { text: `uite ${GIF}` } });
		const img = screen.getByRole('img', { name: 'GIF' });
		expect(img).toHaveAttribute('src', GIF);
	});

	it('keeps the page url out of giphy logs', () => {
		render(RichText, { props: { text: GIF } });
		expect(screen.getByRole('img', { name: 'GIF' })).toHaveAttribute(
			'referrerpolicy',
			'no-referrer'
		);
	});

	it('loads gifs lazily', () => {
		render(RichText, { props: { text: GIF } });
		expect(screen.getByRole('img', { name: 'GIF' })).toHaveAttribute('loading', 'lazy');
	});

	it('leaves a non-giphy link as text', () => {
		const { container } = render(RichText, {
			props: { text: 'vezi https://example.com/x.gif' }
		});
		expect(container.querySelector('img')).toBeNull();
		expect(container.textContent).toContain('https://example.com/x.gif');
	});

	it('hides a gif wrapped in a spoiler until it is revealed', async () => {
		render(RichText, { props: { text: `||${GIF}||` } });
		const button = screen.getByRole('button', { name: /spoiler/i });
		// The <img> renders inside the mask, so it is present but painted out.
		expect(button.querySelector('img')).not.toBeNull();
		await fireEvent.click(button);
		expect(screen.queryByRole('button')).toBeNull();
		expect(screen.getByRole('img', { name: 'GIF' })).toBeInTheDocument();
	});
});
