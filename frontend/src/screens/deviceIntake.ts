// "Suchen": find a device that's already in GLPI by serial number, then
// finish it on the review screen. Files from the USB stick are picked when
// finishing, not here.
import {FindComputerBySerial} from '../../wailsjs/go/main/App';
import type {glpi} from '../../wailsjs/go/models';
import {escapeHtml} from '../html';
import {t} from '../i18n';

export interface DeviceIntakeResult {
  computer: glpi.Computer;
}

export function renderDeviceIntake(content: HTMLElement, asUser: string, onSelectDevice: (result: DeviceIntakeResult) => void) {
  content.innerHTML = `
    <div class="intake-screen">
      <div class="intake-header">
        <div>
          <h1 class="screen-title">${t('intake.title')}</h1>
          <p class="screen-subtitle">${escapeHtml(t('intake.loggedInAs', {user: asUser}))}</p>
        </div>
      </div>

      <section class="card">
        <h2 class="card-title">${t('intake.searchTitle')}</h2>
        <div class="serial-search">
          <input id="serial-input" type="text" placeholder="${t('intake.serial')}" />
          <button id="btn-search" class="submit-btn submit-btn--inline">${t('intake.search')}</button>
        </div>
        <div id="serial-result"></div>
      </section>
    </div>
  `;

  const serialInput = content.querySelector<HTMLInputElement>('#serial-input')!;
  const serialResult = content.querySelector<HTMLDivElement>('#serial-result')!;
  const btnSearch = content.querySelector<HTMLButtonElement>('#btn-search')!;

  function searchSerial() {
    const serial = serialInput.value.trim();
    if (!serial) return;
    serialResult.innerHTML = `<p class="muted">${t('intake.searching')}</p>`;
    FindComputerBySerial(serial)
      .then((computers: glpi.Computer[]) => {
        if (!computers || computers.length === 0) {
          serialResult.innerHTML = `<p class="warning">${t('intake.notFound')}</p>`;
          return;
        }
        serialResult.innerHTML = computers
          .map(
            (c, i) => `
              <div class="device-result">
                <span class="device-result-name">${escapeHtml(c.name ?? '')}</span>
                <span class="device-result-meta">${escapeHtml(c.manufacturer?.name ?? '')} ${escapeHtml(c.model?.name ?? '')}</span>
                <span class="badge">${escapeHtml(c.status?.name ?? t('intake.noStatus'))}</span>
                <button class="submit-btn submit-btn--inline" data-select-index="${i}">${t('intake.next')}</button>
              </div>
            `
          )
          .join('');

        serialResult.querySelectorAll<HTMLButtonElement>('[data-select-index]').forEach((btn) => {
          btn.addEventListener('click', () => onSelectDevice({computer: computers[Number(btn.dataset.selectIndex)]}));
        });
      })
      .catch((err) => {
        serialResult.innerHTML = `<p class="warning">${t('intake.failed')}</p>`;
        console.error(err);
      });
  }

  btnSearch.addEventListener('click', searchSerial);
  serialInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') searchSerial();
  });
  serialInput.focus();
}
