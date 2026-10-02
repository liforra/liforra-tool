// A plain text input that also offers GLPI's existing catalog values as
// type-to-filter suggestions — not a strict <select>: whatever's in the
// input when it loses focus is the value, matched or not, so a technician
// can still enter a value GLPI doesn't have yet (the existing "nicht in
// GLPI" flagging and missing-entries flow downstream already handles that).
import {escapeHtml} from './html';

const MAX_SUGGESTIONS = 30;

export interface ComboboxHandle {
  // Forces a refetch next time the list opens — e.g. after the technician
  // picks a different manufacturer, model suggestions should reflect it.
  invalidate(): void;
}

export function attachCombobox(input: HTMLInputElement, fetchOptions: () => Promise<string[]>): ComboboxHandle {
  let options: string[] | null = null;
  let loading: Promise<string[]> | null = null;
  let activeIndex = -1;
  // Surfaced in the list itself instead of only the console — a technician
  // can't open devtools on a production build, so a silent catch here used
  // to look identical to "nothing matches" from where they're sitting.
  let loadError = '';

  const list = document.createElement('div');
  list.className = 'combobox-list';
  list.hidden = true;
  // Positioned relative to a wrapper so it tracks the input regardless of
  // where attachCombobox is called from in the surrounding markup.
  const wrapper = document.createElement('div');
  wrapper.className = 'combobox-wrapper';
  input.parentElement!.insertBefore(wrapper, input);
  wrapper.appendChild(input);
  wrapper.appendChild(list);

  function ensureOptions(): Promise<string[]> {
    if (options) return Promise.resolve(options);
    if (!loading) {
      loading = fetchOptions()
        .then((v) => {
          options = v;
          loadError = '';
          return v;
        })
        .catch((err) => {
          console.error(err);
          loadError = String(err);
          options = [];
          return options;
        });
    }
    return loading;
  }

  function render(matches: string[], totalLoaded: number) {
    activeIndex = -1;
    if (loadError) {
      list.innerHTML = `<div class="combobox-error">${escapeHtml(loadError)}</div>`;
      list.hidden = false;
      return;
    }
    if (matches.length === 0) {
      // Still shown, not hidden — "0 geladen" vs "12 geladen, 0 passend"
      // are two completely different problems (fetch vs filter), and
      // silence here made them indistinguishable from the outside, twice.
      list.innerHTML = `<div class="combobox-error">(${totalLoaded} geladen, 0 passend)</div>`;
      list.hidden = false;
      return;
    }
    list.innerHTML = matches
      .slice(0, MAX_SUGGESTIONS)
      .map((m, i) => `<button type="button" class="combobox-option" data-i="${i}">${escapeHtml(m)}</button>`)
      .join('');
    list.hidden = false;
  }

  function filterAndRender() {
    const q = input.value.trim().toLowerCase();
    ensureOptions().then((opts) => {
      const matches = q === '' ? opts : opts.filter((o) => o.toLowerCase().includes(q));
      render(matches, opts.length);
    });
  }

  function setActive(i: number) {
    const items = list.querySelectorAll<HTMLButtonElement>('.combobox-option');
    items.forEach((el) => el.classList.remove('combobox-option--active'));
    if (i >= 0 && i < items.length) {
      items[i].classList.add('combobox-option--active');
      items[i].scrollIntoView({block: 'nearest'});
    }
    activeIndex = i;
  }

  function choose(value: string) {
    input.value = value;
    list.hidden = true;
    input.dispatchEvent(new Event('input', {bubbles: true}));
    input.dispatchEvent(new Event('change', {bubbles: true}));
  }

  input.addEventListener('focus', filterAndRender);
  input.addEventListener('input', filterAndRender);

  input.addEventListener('keydown', (e) => {
    const items = list.querySelectorAll<HTMLButtonElement>('.combobox-option');
    if (list.hidden || items.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive(Math.min(activeIndex + 1, items.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(Math.max(activeIndex - 1, 0));
    } else if (e.key === 'Enter' && activeIndex >= 0) {
      e.preventDefault();
      choose(items[activeIndex].textContent ?? '');
    } else if (e.key === 'Escape') {
      list.hidden = true;
    }
  });

  // mousedown (not click) fires before the input's blur handler, so the
  // value is applied before the list disappears out from under the click.
  list.addEventListener('mousedown', (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>('.combobox-option');
    if (btn) {
      e.preventDefault();
      choose(btn.textContent ?? '');
    }
  });

  input.addEventListener('blur', () => {
    // Clicking out keeps whatever text is in the input as-is — this is
    // deliberately not forced to match a known suggestion.
    setTimeout(() => (list.hidden = true), 100);
  });

  return {
    invalidate() {
      options = null;
      loading = null;
    },
  };
}
