import {t} from '../i18n';

const closeIcon =
  '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>';

export interface Choice {
  key: string;
  label: string;
  primary?: boolean;
}

// Resolves to the chosen key, or null for close / Escape. beforeClose sees
// the dialog one last time, e.g. to read checkboxes in bodyHtml.
export function choiceDialog(opts: {
  title: string;
  bodyHtml: string;
  choices: Choice[];
  beforeClose?: (key: string | null, dialog: HTMLElement) => void;
}): Promise<string | null> {
  const overlay = document.querySelector<HTMLDivElement>('#modal-overlay')!;
  overlay.innerHTML = `
    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="dialog-title">
      <div class="modal-header">
        <h2 class="card-title" id="dialog-title"></h2>
        <button class="titlebar-btn" id="btn-dialog-close" aria-label="${t('common.close')}">${closeIcon}</button>
      </div>
      <div>${opts.bodyHtml}</div>
      <div class="modal-actions">
        ${opts.choices
          .map((c, i) => `<button class="submit-btn submit-btn--inline${c.primary ? '' : ' submit-btn--secondary'}" data-choice="${i}"></button>`)
          .join('')}
      </div>
    </div>
  `;
  overlay.querySelector('#dialog-title')!.textContent = opts.title;
  const buttons = overlay.querySelectorAll<HTMLButtonElement>('[data-choice]');
  buttons.forEach((b) => (b.textContent = opts.choices[Number(b.dataset.choice)].label));

  return new Promise((resolve) => {
    function close(result: string | null) {
      opts.beforeClose?.(result, overlay);
      overlay.innerHTML = '';
      document.removeEventListener('keydown', onKey);
      resolve(result);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') close(null);
    }
    document.addEventListener('keydown', onKey);
    overlay.querySelector('#btn-dialog-close')!.addEventListener('click', () => close(null));
    buttons.forEach((b) => b.addEventListener('click', () => close(opts.choices[Number(b.dataset.choice)].key)));
    buttons[0]?.focus();
  });
}

// The "Informationen für TXT Datei Fehlen" prompt, shared by both finish
// flows: go back, finish without the TXT, or create it with the gaps empty.
export function askMissingTxtInfo(missingLabels: string[]): Promise<'back' | 'without' | 'anyway'> {
  const body = document.createElement('p');
  body.className = 'muted';
  body.textContent = t('txt.missing', {list: missingLabels.join(', ')});
  return choiceDialog({
    title: t('txt.title'),
    bodyHtml: body.outerHTML,
    choices: [
      {key: 'back', label: t('common.back')},
      {key: 'without', label: t('txt.without')},
      {key: 'anyway', label: t('txt.anyway'), primary: true},
    ],
  }).then((c) => (c === 'without' || c === 'anyway' ? c : 'back'));
}
