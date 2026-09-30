// "Prüfen": cross-checks the scanned device against itself — does the
// erase certificate on the USB stick actually name the disk installed
// here, does the test report's serial match, is this serial number
// duplicated in GLPI, does the GLPI record's manufacturer/model/type
// roughly agree with what's physically detected. Read-only except for the
// final "Fertig geprüft" button, which records who reviewed the device.
import {CheckDeviceAgainstGLPI, CheckDeviceDocuments, CheckDeviceOS, EjectDrive, FindComputerBySerial, ListUSBDrives, MarkDeviceChecked, RemoveAllWiFiNetworks} from '../../wailsjs/go/main/App';
import type {devicecheck, glpi, hwinfo, usbscan} from '../../wailsjs/go/models';
import {escapeHtml} from '../html';
import {t, type Key} from '../i18n';
import {latestScan, startScan} from '../scan';
import {filterForDevice, scanAllUSBDrives} from './attachments';
import {choiceDialog} from './dialog';

// Maps a devicecheck.Result's Key to its localized label/detail — the Go
// side only ever supplies structured Key+Params, never text (see
// internal/devicecheck's package doc), same split as the rest of the app.
const CHECK_KEYS: Partial<Record<string, {label: Key; detail?: Key}>> = {
  'erase.missing': {label: 'check.erase.missing'},
  'erase.ok': {label: 'check.erase.ok'},
  'erase.failed': {label: 'check.erase.failed', detail: 'check.erase.failedDetail'},
  'erase.diskMismatch': {label: 'check.erase.diskMismatch', detail: 'check.erase.diskMismatchDetail'},
  'erase.countMismatch': {label: 'check.erase.countMismatch', detail: 'check.erase.countMismatchDetail'},
  'test.missing': {label: 'check.test.missing'},
  'test.ok': {label: 'check.test.ok'},
  'test.failed': {label: 'check.test.failed', detail: 'check.test.failedDetail'},
  'test.serialMismatch': {label: 'check.test.serialMismatch', detail: 'check.test.serialMismatchDetail'},
  'glpi.notFound': {label: 'check.glpi.notFound'},
  'glpi.ok': {label: 'check.glpi.ok'},
  'glpi.duplicate': {label: 'check.glpi.duplicate', detail: 'check.glpi.duplicateDetail'},
  'glpi.mismatchManufacturer': {label: 'check.glpi.mismatchManufacturer', detail: 'check.glpi.mismatchDetail'},
  'glpi.mismatchModel': {label: 'check.glpi.mismatchModel', detail: 'check.glpi.mismatchDetail'},
  'glpi.mismatchType': {label: 'check.glpi.mismatchType', detail: 'check.glpi.mismatchDetail'},
  'os.account.ok': {label: 'check.os.account.ok'},
  'os.account.wrongName': {label: 'check.os.account.wrongName', detail: 'check.os.account.wrongNameDetail'},
  'os.account.none': {label: 'check.os.account.none'},
  'os.account.multiple': {label: 'check.os.account.multiple', detail: 'check.os.account.multipleDetail'},
  'os.updates.ok': {label: 'check.os.updates.ok'},
  'os.updates.pending': {label: 'check.os.updates.pending', detail: 'check.os.updates.pendingDetail'},
  'os.updates.unknown': {label: 'check.os.updates.unknown'},
  'os.devices.ok': {label: 'check.os.devices.ok'},
  'os.devices.problem': {label: 'check.os.devices.problem', detail: 'check.os.devices.problemDetail'},
};

function badgeText(status: string): string {
  return status === 'ok' ? '✓' : status === 'warn' ? '⚠' : '✗';
}

function renderRow(r: devicecheck.Result): string {
  const meta = CHECK_KEYS[r.key];
  const label = meta ? t(meta.label) : r.key;
  const detail = meta?.detail ? t(meta.detail, r.params ?? {}) : '';
  return `
    <div class="file-card">
      <div class="file-card-main">
        <span class="file-card-name">${escapeHtml(label)}</span>
        ${detail ? `<span class="file-card-detail">${escapeHtml(detail)}</span>` : ''}
      </div>
      <span class="badge badge--${r.status}">${badgeText(r.status)}</span>
    </div>
  `;
}

function renderRows(container: HTMLElement, results: devicecheck.Result[]) {
  container.innerHTML = results.length ? results.map(renderRow).join('') : `<p class="muted">${t('check.noneYet')}</p>`;
}

export function renderDeviceCheck(content: HTMLElement) {
  content.innerHTML = `
    <div class="intake-screen">
      <div class="intake-header">
        <div>
          <h1 class="screen-title">${t('check.title')}</h1>
          <p class="screen-subtitle">${t('check.subtitle')}</p>
        </div>
        <button class="submit-btn" id="btn-check-usb">${t('check.scanUsb')}</button>
      </div>

      <section class="card">
        <h2 class="card-title">${t('check.section.documents')}</h2>
        <div id="check-drive-row"></div>
        <div id="check-doc-results"><p class="muted">${t('check.noneYet')}</p></div>
      </section>

      <section class="card">
        <h2 class="card-title">${t('check.section.os')}</h2>
        <div id="check-os-results"><p class="muted">${t('check.scanning')}</p></div>
        <button class="submit-btn submit-btn--inline submit-btn--secondary spec-gap" id="btn-wifi-remove">${t('check.wifi.remove')}</button>
        <p class="muted" id="wifi-remove-status"></p>
      </section>

      <section class="card">
        <h2 class="card-title">${t('check.section.glpi')}</h2>
        <div id="check-glpi-results"><p class="muted">${t('check.noneYet')}</p></div>
      </section>

      <div>
        <button class="submit-btn" id="btn-mark-done" disabled>${t('check.markDone')}</button>
        <p class="muted" id="mark-done-note">${t('check.needsSingleMatch')}</p>
        <p class="error-message" id="mark-done-error"></p>
      </div>
    </div>
  `;

  const docResults = content.querySelector<HTMLDivElement>('#check-doc-results')!;
  const driveRow = content.querySelector<HTMLDivElement>('#check-drive-row')!;
  const osResults = content.querySelector<HTMLDivElement>('#check-os-results')!;
  const glpiResults = content.querySelector<HTMLDivElement>('#check-glpi-results')!;
  const btnUsb = content.querySelector<HTMLButtonElement>('#btn-check-usb')!;
  const btnDone = content.querySelector<HTMLButtonElement>('#btn-mark-done')!;
  const doneNote = content.querySelector<HTMLParagraphElement>('#mark-done-note')!;
  const doneError = content.querySelector<HTMLParagraphElement>('#mark-done-error')!;

  let matchedComputerID = 0;

  function setDoneEnabled(computer: glpi.Computer | null) {
    matchedComputerID = computer?.id ?? 0;
    btnDone.disabled = matchedComputerID === 0;
    doneNote.hidden = matchedComputerID !== 0;
  }

  async function runGLPICheck(info: hwinfo.Info) {
    if (!info.serialNumber) {
      renderRows(glpiResults, []);
      setDoneEnabled(null);
      return;
    }
    try {
      const matches = await FindComputerBySerial(info.serialNumber);
      const results = await CheckDeviceAgainstGLPI(info, matches ?? []);
      renderRows(glpiResults, results);
      setDoneEnabled(matches?.length === 1 ? matches[0] : null);
    } catch (err) {
      glpiResults.innerHTML = `<p class="warning">${escapeHtml(t('check.glpi.notFound'))}</p>`;
      console.error(err);
      setDoneEnabled(null);
    }
  }

  // A physical stick can show up as several removable partitions (a
  // Ventoy stick has three) — show all of them, one eject action for all,
  // since ejecting any one volume of a USB device ejects the whole thing.
  function renderDriveRow(drives: usbscan.Drive[]) {
    if (drives.length === 0) {
      driveRow.innerHTML = `<p class="muted">${t('check.drive.none')}</p>`;
      return;
    }
    const label = drives.map((d) => d.label || d.path).join(', ');
    driveRow.innerHTML = `
      <div class="file-card">
        <div class="file-card-main">
          <span class="file-card-name">${escapeHtml(t('check.drive.found', {label}))}</span>
        </div>
        <button class="submit-btn submit-btn--inline submit-btn--secondary" id="btn-eject">${t('check.drive.eject')}</button>
      </div>
    `;
    driveRow.querySelector<HTMLButtonElement>('#btn-eject')!.addEventListener('click', async (e) => {
      const btn = e.currentTarget as HTMLButtonElement;
      btn.disabled = true;
      btn.textContent = t('check.drive.ejecting');
      try {
        await Promise.all(drives.map((d) => EjectDrive(d.path)));
        btn.textContent = t('check.drive.ejected');
      } catch (err) {
        btn.textContent = t('check.drive.ejectErr');
        btn.disabled = false;
        console.error(err);
      }
    });
  }

  async function runDocumentCheck(info: hwinfo.Info) {
    btnUsb.disabled = true;
    btnUsb.textContent = t('check.scanning');
    try {
      const drives = (await ListUSBDrives()) ?? [];
      renderDriveRow(drives);
      const diskSerials = (info.disks ?? []).map((d) => d.serial);
      const files = drives.length ? filterForDevice(await scanAllUSBDrives(), info.serialNumber, diskSerials) : [];
      const results = await CheckDeviceDocuments(info, files);
      renderRows(docResults, results);
    } catch (err) {
      docResults.innerHTML = `<p class="warning">${escapeHtml(t('check.erase.missing'))}</p>`;
      console.error(err);
    } finally {
      btnUsb.disabled = false;
      btnUsb.textContent = t('check.scanUsb');
    }
  }

  // Independent of the hardware/USB scan and not re-run by "USB-Stick
  // prüfen" — a Windows Update search is slow enough that re-running it on
  // every rescan click would just be wasted waiting.
  CheckDeviceOS()
    .then((results) => renderRows(osResults, results))
    .catch((err) => {
      osResults.innerHTML = `<p class="warning">${escapeHtml(t('check.os.updates.unknown'))}</p>`;
      console.error(err);
    });

  latestScan().then((info) => {
    runGLPICheck(info);
    runDocumentCheck(info);
  });

  btnUsb.addEventListener('click', () => {
    startScan()
      .then((info) => {
        runGLPICheck(info);
        return runDocumentCheck(info);
      })
      .catch((err) => console.error(err));
  });

  const btnWifiRemove = content.querySelector<HTMLButtonElement>('#btn-wifi-remove')!;
  const wifiRemoveStatus = content.querySelector<HTMLParagraphElement>('#wifi-remove-status')!;

  btnWifiRemove.addEventListener('click', async () => {
    const confirmed = await choiceDialog({
      title: t('check.wifi.confirmTitle'),
      bodyHtml: `<p class="warning">${escapeHtml(t('check.wifi.confirmBody'))}</p>`,
      choices: [
        {key: 'cancel', label: t('common.back')},
        {key: 'remove', label: t('check.wifi.confirmYes'), primary: true},
      ],
    });
    if (confirmed !== 'remove') return;

    wifiRemoveStatus.textContent = '';
    btnWifiRemove.disabled = true;
    const original = btnWifiRemove.textContent;
    btnWifiRemove.textContent = t('check.wifi.removing');
    try {
      const result = await RemoveAllWiFiNetworks();
      wifiRemoveStatus.textContent =
        result.failed && result.failed.length > 0
          ? t('check.wifi.resultWithFailed', {removed: result.removed, failed: result.failed.length})
          : t('check.wifi.result', {removed: result.removed});
    } catch (err) {
      wifiRemoveStatus.textContent = t('check.wifi.error');
      console.error(err);
    } finally {
      btnWifiRemove.disabled = false;
      btnWifiRemove.textContent = original;
    }
  });

  btnDone.addEventListener('click', async () => {
    doneError.textContent = '';
    btnDone.disabled = true;
    const original = btnDone.textContent;
    btnDone.textContent = t('check.markDoneBusy');
    try {
      await MarkDeviceChecked(matchedComputerID);
      btnDone.textContent = t('check.markDoneOk');
    } catch (err) {
      doneError.textContent = t('check.markDoneErr');
      console.error(err);
      btnDone.textContent = original;
      btnDone.disabled = false;
    }
  });
}
