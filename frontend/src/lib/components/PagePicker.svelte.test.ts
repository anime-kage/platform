import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import PagePicker from './PagePicker.svelte';

const openPicker = async () => {
	const trigger = screen.getByRole('button', { name: /alege altă pagină/i });
	await fireEvent.click(trigger);
	return trigger;
};

describe('PagePicker — the trigger', () => {
	it('reads as the page line it replaced', () => {
		render(PagePicker, { props: { page: 3, pages: 32 } });
		const trigger = screen.getByRole('button', { name: /alege altă pagină/i });
		expect(trigger.textContent?.replace(/\s+/g, ' ')).toContain('pagina 3 din 32');
	});

	it('announces itself as collapsed until opened', async () => {
		render(PagePicker, { props: { page: 1, pages: 5 } });
		const trigger = screen.getByRole('button', { name: /alege altă pagină/i });
		expect(trigger).toHaveAttribute('aria-expanded', 'false');
		await fireEvent.click(trigger);
		expect(trigger).toHaveAttribute('aria-expanded', 'true');
	});

	it('toggles shut on a second click', async () => {
		render(PagePicker, { props: { page: 1, pages: 5 } });
		const trigger = await openPicker();
		expect(screen.getByRole('dialog')).toBeInTheDocument();
		await fireEvent.click(trigger);
		expect(screen.queryByRole('dialog')).toBeNull();
	});
});

describe('PagePicker — choosing a page', () => {
	it('renders buttons and reports the choice when paging is component state', async () => {
		const onselect = vi.fn();
		render(PagePicker, { props: { page: 1, pages: 4, onselect } });
		await openPicker();
		await fireEvent.click(screen.getByRole('button', { name: '3' }));
		expect(onselect).toHaveBeenCalledWith(3);
		// and it closes behind you
		expect(screen.queryByRole('dialog')).toBeNull();
	});

	it('renders links when paging lives in the URL', async () => {
		// A link that goes nowhere is a lie to anyone middle-clicking, so the
		// two modes must not be confused.
		render(PagePicker, { props: { page: 1, pages: 3, hrefFor: (n: number) => `?page=${n}` } });
		await openPicker();
		expect(screen.getByRole('link', { name: '2' })).toHaveAttribute('href', '?page=2');
		expect(screen.queryByRole('button', { name: '2' })).toBeNull();
	});

	it('marks the current page for assistive tech', async () => {
		render(PagePicker, { props: { page: 2, pages: 4, onselect: vi.fn() } });
		await openPicker();
		expect(screen.getByRole('button', { name: '2' })).toHaveAttribute('aria-current', 'page');
		expect(screen.getByRole('button', { name: '3' })).not.toHaveAttribute('aria-current');
	});

	it('lists every page', async () => {
		render(PagePicker, { props: { page: 1, pages: 7, onselect: vi.fn() } });
		await openPicker();
		for (const n of ['1', '2', '3', '4', '5', '6', '7']) {
			expect(screen.getByRole('button', { name: n })).toBeInTheDocument();
		}
	});
});

describe('PagePicker — the jump field past 40 pages', () => {
	it('is absent while the grid is still scannable', async () => {
		render(PagePicker, { props: { page: 1, pages: 40, onselect: vi.fn() } });
		await openPicker();
		expect(screen.queryByLabelText(/sari la pagina/i)).toBeNull();
	});

	it('appears once a wall of numbers stops being scannable', async () => {
		render(PagePicker, { props: { page: 1, pages: 41, onselect: vi.fn() } });
		await openPicker();
		expect(screen.getByLabelText(/sari la pagina/i)).toBeInTheDocument();
	});

	it('jumps to the typed page', async () => {
		const onselect = vi.fn();
		render(PagePicker, { props: { page: 1, pages: 60, onselect } });
		await openPicker();
		const input = screen.getByLabelText(/sari la pagina/i);
		await fireEvent.input(input, { target: { value: '42' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Sari' }));
		expect(onselect).toHaveBeenCalledWith(42);
	});

	it('constrains the field so the browser blocks an out-of-range page', async () => {
		render(PagePicker, { props: { page: 1, pages: 60, onselect: vi.fn() } });
		await openPicker();
		const input = screen.getByLabelText(/sari la pagina/i);
		expect(input).toHaveAttribute('min', '1');
		expect(input).toHaveAttribute('max', '60');
	});

	it.each(['999', '0', '-5'])('never submits %s', async (typed) => {
		// min/max make the form invalid, so onsubmit does not fire at all —
		// the out-of-range value never reaches go().
		const onselect = vi.fn();
		render(PagePicker, { props: { page: 1, pages: 60, onselect } });
		await openPicker();
		await fireEvent.input(screen.getByLabelText(/sari la pagina/i), {
			target: { value: typed }
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Sari' }));
		expect(onselect).not.toHaveBeenCalled();
	});

	it.each([
		['999', 60],
		['0', 1],
		['-5', 1],
		['', 1]
	])('clamps %s to %i if a submit gets through anyway', async (typed, expected) => {
		// Dispatching submit directly bypasses constraint validation, which is
		// what an older engine or a scripted submit would do. The clamp inside
		// go() is the backstop for that, so it is worth holding in place.
		const onselect = vi.fn();
		const { container } = render(PagePicker, { props: { page: 1, pages: 60, onselect } });
		await openPicker();
		await fireEvent.input(screen.getByLabelText(/sari la pagina/i), {
			target: { value: typed }
		});
		await fireEvent.submit(container.querySelector('form')!);
		expect(onselect).toHaveBeenCalledWith(expected);
	});
});

describe('PagePicker — dismissing', () => {
	it('closes on Escape', async () => {
		render(PagePicker, { props: { page: 1, pages: 5, onselect: vi.fn() } });
		await openPicker();
		await fireEvent.keyDown(window, { key: 'Escape' });
		expect(screen.queryByRole('dialog')).toBeNull();
	});

	it('closes on a pointer press outside it', async () => {
		render(PagePicker, { props: { page: 1, pages: 5, onselect: vi.fn() } });
		await openPicker();
		await fireEvent.pointerDown(document.body);
		expect(screen.queryByRole('dialog')).toBeNull();
	});

	it('stays open on a press inside it', async () => {
		render(PagePicker, { props: { page: 1, pages: 5, onselect: vi.fn() } });
		await openPicker();
		const dialog = screen.getByRole('dialog');
		await fireEvent.pointerDown(dialog);
		expect(screen.getByRole('dialog')).toBeInTheDocument();
	});
});
