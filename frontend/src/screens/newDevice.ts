// "Neues Gerät": scan this PC and take it from no GLPI entry at all to
// fully finished in one page — Computer record with components, Zusatzdaten,
// documents from the USB stick, the generated spec-sheet TXT, and status
// "Einsetzbar". Every question (missing GLPI entries, missing TXT info) is
// asked before anything is written.
import {
  CheckNewDevice,
  ChooseDestinationDirectory,
  CopyUSBFilesTo,
  CreateFullComputer,
  FindComputerBySerial,
  FinishDevice,
  GenerateSpecSheetTXT,
  GetAdministrator,
  LookupLenovoSpecs,
  SaveTextFile,
  UpdateFullComputer,
} from '../../wailsjs/go/main/App';
import {glpi, main} from '../../wailsjs/go/models';
import type {hwinfo, usbscan} from '../../wailsjs/go/models';
import {escapeHtml} from '../html';
import {askMissingTxtInfo, choiceDialog} from './dialog';
import {isLenovo, renderSpecSheetForm, specDefaults, type RequiredKey} from './specSheet';
import {chooseUSBFiles, hasSpecSheet, specSheetAttachment, toAttachments} from './attachments';
import {latestScan, startScan} from '../scan';
import {t} from '../i18n';

const icons: Record<string, string> = {
  cpu: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20v2"/><path d="M12 2v2"/><path d="M17 20v2"/><path d="M17 2v2"/><path d="M2 12h2"/><path d="M2 17h2"/><path d="M2 7h2"/><path d="M20 12h2"/><path d="M20 17h2"/><path d="M20 7h2"/><path d="M7 20v2"/><path d="M7 2v2"/><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="8" y="8" width="8" height="8" rx="1"/></svg>',
  memory: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 19v-3"/><path d="M10 19v-3"/><path d="M14 19v-3"/><path d="M18 19v-3"/><path d="M8 11V9"/><path d="M16 11V9"/><path d="M12 11V9"/><path d="M2 15h20"/><path d="M2 15V7a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v8"/></svg>',
  disk: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="22" x2="2" y1="12" y2="12"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/><line x1="6" x2="6.01" y1="16" y2="16"/><line x1="10" x2="10.01" y1="16" y2="16"/></svg>',
  gpu: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="12" x="2" y="6" rx="2"/><circle cx="7" cy="12" r="2"/><path d="M14 8v.01"/><path d="M18 8v.01"/><path d="M14 12v.01"/><path d="M18 12v.01"/><path d="M14 16v.01"/><path d="M18 16v.01"/></svg>',
};

const closeIcon =
  '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>';

const chevronIcon =
  '<svg class="details-chevron" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>';

function componentCard(kind: keyof typeof icons, title: string, fieldsHtml: string, id = ''): string {
  return `
    <div class="card component-card"${id ? ` id="${id}"` : ''}>
      <h3 class="card-title">${icons[kind]} ${title}</h3>
      ${fieldsHtml}
    </div>
  `;
}

// Shown when GLPI lacks one or more entries the device needs. Only
// administrator mode offers to create them; otherwise the technician has to
// correct the values (or ask an administrator). Resolves true to create them
// and continue.
function askMissingEntries(missing: glpi.MissingEntry[], isAdmin: boolean): Promise<boolean> {
  const overlay = document.querySelector<HTMLDivElement>('#modal-overlay')!;
  const seen = new Set<string>();
  const rows = missing
    .filter((m) => {
      const k = `${m.kind}|${m.name}`;
      if (seen.has(k)) return false;
      seen.add(k);
      return true;
    })
    .map(
      (m) => `
        <div class="file-card">
          <div class="file-card-main">
            <span class="file-card-kind">${escapeHtml(m.kind)}</span>
            <span class="file-card-name">${escapeHtml(m.name)}</span>
          </div>
        </div>
      `
    )
    .join('');

  overlay.innerHTML = `
    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="missing-title">
      <div class="modal-header">
        <h2 class="card-title" id="missing-title">${t('nd.missing.title')}</h2>
        <button class="titlebar-btn" id="btn-missing-close" aria-label="${t('common.close')}">${closeIcon}</button>
      </div>
      <p class="muted">${t('nd.missing.intro')}</p>
      <div>${rows}</div>
      ${isAdmin ? '' : `<p class="muted">${t('nd.missing.nonAdmin')}</p>`}
      <div class="modal-actions">
        <button class="submit-btn submit-btn--inline submit-btn--secondary" id="btn-missing-cancel">${isAdmin ? t('nd.missing.cancel') : t('common.close')}</button>
        ${isAdmin ? `<button class="submit-btn submit-btn--inline" id="btn-missing-add">${t('nd.missing.add')}</button>` : ''}
      </div>
    </div>
  `;

  return new Promise((resolve) => {
    function close(result: boolean) {
      overlay.innerHTML = '';
      document.removeEventListener('keydown', onKey);
      resolve(result);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') close(false);
    }
    document.addEventListener('keydown', onKey);
    overlay.querySelector('#btn-missing-close')!.addEventListener('click', () => close(false));
    overlay.querySelector('#btn-missing-cancel')!.addEventListener('click', () => close(false));
    overlay.querySelector('#btn-missing-add')?.addEventListener('click', () => close(true));
    overlay.querySelector<HTMLButtonElement>(isAdmin ? '#btn-missing-add' : '#btn-missing-cancel')!.focus();
  });
}

// The prefixes this GLPI's DeviceHardDrive catalog uses.
const diskKinds = ['SATA SSD', 'M.2 SATA SSD', 'mSATA SSD', 'M.2 NVME SSD', 'SATA HDD'];

function diskKindSelect(id: string, detected: string): string {
  const options = diskKinds.includes(detected) || !detected ? diskKinds : [detected, ...diskKinds];
  return `
    <div class="field">
      <label for="${id}">${t('nd.diskKind')}</label>
      <select id="${id}">
        ${options.map((k) => `<option value="${escapeHtml(k)}"${k === detected ? ' selected' : ''}>${escapeHtml(k)}</option>`).join('')}
      </select>
    </div>
  `;
}

function field(label: string, id: string, value: string | number, extra = ''): string {
  return `
    <div class="field">
      <label for="${id}">${label}</label>
      <input id="${id}" type="text" value="${escapeHtml(String(value ?? ''))}" ${extra} />
    </div>
  `;
}

// Where a spec-sheet value comes from on this page, for the ones that are
// device fields rather than TXT extras — used to jump to the right input.
const deviceFieldFor: Partial<Record<RequiredKey, string>> = {
  sku: 'f-name',
  geraetetyp: 'f-type',
  produktart: 'f-type',
  marke: 'f-manufacturer',
  modell: 'f-model',
  prozessor: 'f-cpu',
  prozessorgeschwindigkeit: 'f-cpu',
  ram: 'mem-cap-0',
  ramGeschwindigkeit: 'mem-speed-0',
  ssd: 'disk-cap-0',
  grafikeinheit: 'f-gpu',
  betriebssystem: 'f-osname',
};

// Shown right after a scan, before any device info is displayed, whenever
// GLPI already has a Computer with this serial. Resolves to the existing
// Computer to edit it, or null to create a new one regardless.
async function askExistingDevice(existing: glpi.Computer[]): Promise<glpi.Computer | null> {
  // Multiple hits for one serial is a pre-existing data problem, not
  // something to solve here — just offer the first, same as any other
  // "existing device" match.
  const computer = existing[0];
  const rows = existing
    .map(
      (c) => `
      <div class="file-card">
        <div class="file-card-main">
          <span class="file-card-name">${escapeHtml(c.name)}</span>
          <span class="file-card-detail">${escapeHtml(c.serial)}</span>
        </div>
      </div>`
    )
    .join('');

  const key = await choiceDialog({
    title: t('nd.exists.title'),
    bodyHtml: `<p class="muted">${t('nd.exists.intro')}</p><div>${rows}</div>`,
    choices: [
      {key: 'new', label: t('nd.exists.new')},
      {key: 'edit', label: t('nd.exists.edit'), primary: true},
    ],
  });
  return key === 'edit' ? computer : null;
}

export function renderNewDevice(content: HTMLElement, asUser: string) {
  content.innerHTML = `
    <div class="intake-screen intake-screen--wide">
      <div class="intake-header">
        <div>
          <h1 class="screen-title">${t('nd.title')}</h1>
          <p class="screen-subtitle">${escapeHtml(t('nd.loggedInAs', {user: asUser}))}</p>
        </div>
        <div class="intake-header-actions">
          <button class="submit-btn submit-btn--secondary" id="btn-txt-only">${t('nd.txtOnly')}</button>
          <button class="submit-btn submit-btn--secondary" id="btn-copy-usb">${t('nd.copyUsb')}</button>
          <button class="submit-btn" id="btn-detect">${t('nd.scan')}</button>
        </div>
      </div>
      <p class="muted" id="nd-tools-status"></p>
      <div id="detect-status"></div>
      <div id="device-form" class="stack"></div>
    </div>
  `;

  const btnDetect = content.querySelector<HTMLButtonElement>('#btn-detect')!;
  const detectStatus = content.querySelector<HTMLDivElement>('#detect-status')!;
  const form = content.querySelector<HTMLDivElement>('#device-form')!;
  const toolsStatus = content.querySelector<HTMLParagraphElement>('#nd-tools-status')!;

  // --- Standalone actions: no GLPI involvement at all ---
  let latestSpecTextFn: (() => Promise<string>) | null = null;
  let latestDeviceName = '';

  content.querySelector('#btn-txt-only')!.addEventListener('click', async () => {
    if (!latestSpecTextFn) {
      toolsStatus.textContent = t('nd.txtOnly.noScan');
      return;
    }
    toolsStatus.textContent = '';
    const text = await latestSpecTextFn();
    const saved = await SaveTextFile(`${latestDeviceName || 'geraet'}.txt`, text).catch((err) => {
      console.error(err);
      toolsStatus.textContent = t('nd.txtOnly.failed');
      return '';
    });
    if (saved) toolsStatus.textContent = t('nd.txtOnly.saved', {path: saved});
  });

  content.querySelector('#btn-copy-usb')!.addEventListener('click', async () => {
    toolsStatus.textContent = '';
    const dir = await ChooseDestinationDirectory().catch(() => '');
    if (!dir) return;
    toolsStatus.textContent = t('nd.copyUsb.running');
    try {
      const result = await CopyUSBFilesTo(dir);
      toolsStatus.textContent =
        result.copied === 0
          ? t('nd.copyUsb.none')
          : t(result.copied === 1 ? 'nd.copyUsb.doneOne' : 'nd.copyUsb.doneMany', {n: result.copied, path: dir});
      if (result.warnings && result.warnings.length) console.error('CopyUSBFilesTo warnings:', result.warnings);
    } catch (err) {
      console.error(err);
      toolsStatus.textContent = t('nd.copyUsb.failed');
    }
  });

  function showScan(scan: Promise<hwinfo.Info>) {
    btnDetect.disabled = true;
    btnDetect.textContent = t('nd.scanning');
    detectStatus.innerHTML = `<p class="muted">${t('nd.scanningStatus')}</p>`;
    form.innerHTML = '';

    scan
      .then(async (info: hwinfo.Info) => {
        btnDetect.disabled = false;
        btnDetect.textContent = t('nd.rescan');
        detectStatus.innerHTML = '';

        let existing: glpi.Computer | null = null;
        if (info.serialNumber) {
          try {
            const matches = await FindComputerBySerial(info.serialNumber);
            if (matches && matches.length > 0) existing = await askExistingDevice(matches);
          } catch (err) {
            console.error(err);
          }
        }
        renderForm(info, existing ?? undefined);
      })
      .catch((err) => {
        btnDetect.disabled = false;
        btnDetect.textContent = t('nd.scan');
        detectStatus.innerHTML = `<p class="warning">${t('nd.scanFailed')}</p>`;
        console.error(err);
      });
  }

  btnDetect.addEventListener('click', () => showScan(startScan()));
  // Started when the app opened, so this is usually already done.
  showScan(latestScan());

  function renderForm(info: hwinfo.Info, existing?: glpi.Computer) {
    // GLPI is authoritative over the freshly detected hardware info whenever
    // both exist — an existing record reflects what a human already
    // confirmed (or corrected) in GLPI, the scan is just this run's guess.
    const pick = (fromGlpi: string | undefined, detected: string) => (fromGlpi ? fromGlpi : detected);
    const name = existing?.name ?? '';
    const type = pick(existing?.type?.name, info.deviceType);
    const manufacturer = pick(existing?.manufacturer?.name, info.manufacturer);
    const model = pick(existing?.model?.name, info.model);
    const serial = pick(existing?.serial, info.serialNumber);

    const memoryFields = (info.memory ?? [])
      .map(
        (m, i) => `
        <div class="grid-3">
          ${field(t('nd.capacityGB'), `mem-cap-${i}`, Math.round(m.capacityGB * 10) / 10)}
          ${field(t('nd.type'), `mem-type-${i}`, m.ddrType)}
          ${field(t('nd.speedMHz'), `mem-speed-${i}`, m.speedMHz || '')}
        </div>
      `
      )
      .join('');

    const diskCards = (info.disks ?? [])
      .map(
        (d, i) => `
        <div class="card component-card" id="disk-card-${i}">
          <h3 class="card-title">${icons.disk} ${t('nd.disk', {n: i + 1})}</h3>
          <div class="grid-2">
            ${diskKindSelect(`disk-kind-${i}`, d.kind)}
            ${field(t('nd.capacityGB'), `disk-cap-${i}`, Math.round(d.capacityGB))}
          </div>
        </div>
      `
      )
      .join('');

    const batteryHealth = (info.batteries ?? [])
      .filter((b) => b.designCapacityMWh > 0)
      .slice(0, 2)
      .map((b) => Math.round((b.fullChargeCapacityMWh / b.designCapacityMWh) * 100));

    form.innerHTML = `
      ${
        existing
          ? `<div class="card" id="editing-banner"><p class="muted">${escapeHtml(t('nd.editing', {name: existing.name, id: String(existing.id)}))}</p></div>`
          : ''
      }
      <fieldset id="device-fields" class="stack plain-fieldset">
        <div class="grid-2">
          <div class="card component-card" id="general-card">
            <h3 class="card-title">${t('nd.general')}</h3>
            ${field(t('nd.name'), 'f-name', name, `placeholder="${t('nd.namePlaceholder')}"`)}
            <div class="grid-2">
              ${field(t('nd.type'), 'f-type', type)}
              ${field(t('nd.serial'), 'f-serial', serial)}
            </div>
            <div class="grid-2">
              ${field(t('nd.manufacturer'), 'f-manufacturer', manufacturer)}
              ${field(t('nd.model'), 'f-model', model)}
            </div>
          </div>

          <div class="card component-card" id="os-card">
            <h3 class="card-title">${t('nd.os')}</h3>
            ${field(t('nd.osName'), 'f-osname', info.osName)}
            ${field(t('nd.osVersion'), 'f-osversion', info.osVersion)}
          </div>
        </div>

        <div class="card component-card" id="cpu-card">
          <h3 class="card-title">${icons.cpu} ${t('nd.cpu')}</h3>
          ${field(t('nd.model'), 'f-cpu', info.cpuModel)}
        </div>

        ${info.memory && info.memory.length ? componentCard('memory', t('nd.memory'), memoryFields, 'memory-card') : ''}

        ${diskCards}

        <div class="card component-card" id="gpu-card">
          <h3 class="card-title">${icons.gpu} ${t('nd.gpu')}</h3>
          ${field(t('nd.model'), 'f-gpu', info.gpuModel)}
        </div>
      </fieldset>

      <section class="card">
        <h3 class="card-title">${t('nd.extraTitle')}</h3>
        <div class="field">
          <label for="f-maengel">${t('nd.defects')}</label>
          <textarea id="f-maengel" rows="2"></textarea>
        </div>
        <div class="grid-2">
          ${batteryHealth
            .map(
              (h, i) => `
            <div class="field">
              <label for="f-akku${i + 1}">${t(i ? 'nd.battery2' : 'nd.battery')}</label>
              <input id="f-akku${i + 1}" type="number" min="0" max="100" value="${h}" />
            </div>`
            )
            .join('')}
          <div class="field">
            <label for="f-osaktiv">${t('nd.osActivated')}</label>
            <select id="f-osaktiv">
              <option value="0">${t('common.no')}</option>
              <option value="1"${info.osActivated ? ' selected' : ''}>${t('common.yes')}</option>
            </select>
          </div>
        </div>
      </section>

      <details class="card" id="spec-card">
        <summary>
          <span class="card-title">${t('nd.spec.title')}</span>
          <span class="badge" id="spec-status"></span>
          ${chevronIcon}
        </summary>
        <p class="muted spec-note">${t('nd.spec.note')}</p>
        <div id="spec-form" class="spec-form"></div>
        <button class="submit-btn submit-btn--inline submit-btn--secondary spec-gap" id="btn-txt-preview">${t('nd.preview')}</button>
        <pre id="txt-preview" class="txt-preview"></pre>
      </details>

      <div>
        <button class="submit-btn" id="btn-create">${escapeHtml(t('nd.create'))}</button>
        <p class="error-message" id="create-error"></p>
      </div>
    `;

    const $ = <T extends HTMLElement>(id: string) => form.querySelector<T>(`#${id}`);
    const val = (id: string) => ($<HTMLInputElement>(id)?.value ?? '').trim();
    const num = (id: string) => Number($<HTMLInputElement>(id)?.value) || 0;

    const deviceFields = $<HTMLFieldSetElement>('device-fields')!;
    const btnCreate = $<HTMLButtonElement>('btn-create')!;
    const createError = $<HTMLParagraphElement>('create-error')!;
    const specCard = $<HTMLDetailsElement>('spec-card')!;
    const specStatus = $<HTMLSpanElement>('spec-status')!;

    function collectInput(): main.NewDeviceInput {
      return main.NewDeviceInput.createFrom({
        name: val('f-name'),
        serial: val('f-serial'),
        type: val('f-type'),
        manufacturer: val('f-manufacturer'),
        model: val('f-model'),
        osName: val('f-osname'),
        osVersion: val('f-osversion'),
        cpuModel: val('f-cpu'),
        gpuModel: val('f-gpu'),
        memory: (info.memory ?? []).map((m, i) =>
          main.MemoryModuleInput.createFrom({
            capacityGB: num(`mem-cap-${i}`),
            ddrType: val(`mem-type-${i}`),
            speedMHz: num(`mem-speed-${i}`),
            formFactor: m.formFactor,
          })
        ),
        disks: (info.disks ?? []).map((d, i) =>
          main.DiskInput.createFrom({
            kind: $<HTMLSelectElement>(`disk-kind-${i}`)?.value ?? d.kind,
            capacityGB: num(`disk-cap-${i}`),
          })
        ),
      });
    }

    // Spec-sheet values from the device fields, plus GLPI's own spelling of
    // manufacturer/model/type once the Computer exists.
    function specBase(computer?: glpi.Computer): ReturnType<typeof specDefaults> {
      const input = collectInput();
      const c =
        computer ??
        glpi.Computer.createFrom({
          name: input.name,
          serial: input.serial,
          manufacturer: {name: input.manufacturer},
          model: {name: input.model},
          type: {name: input.type},
        });
      return specDefaults({computer: c, scan: {info, input}});
    }

    const specForm = renderSpecSheetForm($<HTMLDivElement>('spec-form')!, specBase(), false);

    // Lenovo's own published spec sheet knows weight/dimensions/ports this
    // machine's own hardware scan can't detect — see LookupLenovoSpecs.
    // Quietly best-effort: only fills fields still empty, never blocks.
    const initialSpec = specBase();
    if (isLenovo(initialSpec.marke) && initialSpec.modell) {
      LookupLenovoSpecs(initialSpec.modell)
        .then((fields) => specForm.fillEmpty(fields))
        .catch((err) => console.error(err));
    }

    // --- TXT completeness, shown on the collapsed section ---
    function updateSpecStatus() {
      const n = specForm.missing(specBase()).length;
      specStatus.textContent = n === 0 ? t('nd.spec.complete') : t(n === 1 ? 'nd.spec.missingOne' : 'nd.spec.missingMany', {n});
      specStatus.classList.toggle('badge--ok', n === 0);
    }
    form.addEventListener('input', updateSpecStatus);
    form.addEventListener('change', updateSpecStatus);
    updateSpecStatus();

    $('btn-txt-preview')!.addEventListener('click', () => {
      GenerateSpecSheetTXT(specForm.read(specBase()))
        .then((text) => ($<HTMLPreElement>('txt-preview')!.textContent = text))
        .catch((err) => console.error(err));
    });

    // Feeds the always-visible "Nur TXT erstellen" header button — reads
    // whatever's currently in the form, same as the preview above.
    latestSpecTextFn = () => GenerateSpecSheetTXT(specForm.read(specBase(created ?? existing)));
    latestDeviceName = name || info.model || 'geraet';

    // --- "Nicht in GLPI" badges ---
    function flagMissing(missing: glpi.MissingEntry[]) {
      form.querySelectorAll('.catalog-flag').forEach((el) => el.remove());
      const keys = new Set(missing.map((m) => m.key));
      const flag = (id: string, ...cardKeys: string[]) => {
        const card = $(id);
        if (card && cardKeys.some((k) => keys.has(k))) {
          card.insertAdjacentHTML('beforeend', `<span class="catalog-flag">${t('nd.notInGlpi')}</span>`);
        }
      };
      flag('general-card', 'manufacturer', 'model', 'type');
      flag('os-card', 'os', 'osVersion');
      flag('cpu-card', 'cpu');
      flag('gpu-card', 'gpu');
      flag('memory-card', ...(info.memory ?? []).map((_, i) => `memory-${i}`));
      (info.disks ?? []).forEach((_, i) => flag(`disk-card-${i}`, `disk-${i}`));
    }

    CheckNewDevice(collectInput())
      .then((missing) => flagMissing(missing ?? []))
      .catch((err) => console.error(err));

    // --- Create & finish ---
    // Set once the Computer exists, so a retry after a failed finish doesn't
    // create a second one. The device fields are locked from then on.
    let created: glpi.Computer | null = null;
    let createWarnings: string[] = [];
    // The USB files chosen at submit; kept for a retry so the technician
    // isn't asked twice.
    let files: usbscan.FoundFile[] | null = null;

    function setBusy(label: string | null) {
      btnCreate.disabled = label !== null;
      btnCreate.textContent = label ?? (created ? t('nd.retryFinish') : t('nd.create'));
    }

    function focusField(el: HTMLElement | null) {
      if (!el) return;
      if (specCard.contains(el)) specCard.open = true;
      el.scrollIntoView({block: 'center'});
      el.focus();
    }

    btnCreate.addEventListener('click', async () => {
      createError.textContent = '';
      const input = collectInput();

      if (!input.name) {
        createError.textContent = t('nd.errName');
        focusField($('f-name'));
        return;
      }

      setBusy(t('nd.busy.check'));
      try {
        if (!created || existing) {
          const [missing, isAdmin] = await Promise.all([CheckNewDevice(input), GetAdministrator()]);
          if (missing && missing.length > 0) {
            flagMissing(missing);
            setBusy(null);
            if (!(await askMissingEntries(missing, isAdmin))) return;
          }
        }

        if (!files) {
          setBusy(t('nd.busy.usb'));
          const chosen = await chooseUSBFiles(input.name, input.serial, (info.disks ?? []).map((d) => d.serial));
          if (chosen === null) {
            setBusy(null);
            return;
          }
          files = chosen;
        }

        // A spec sheet from the stick replaces the generated one.
        let generateTxt = !hasSpecSheet(files);
        const missingTxt = generateTxt ? specForm.missing(specBase(created ?? undefined)) : [];
        if (missingTxt.length > 0) {
          setBusy(null);
          const choice = await askMissingTxtInfo(missingTxt.map((m) => m.label));
          if (choice === 'back') {
            const first = missingTxt[0];
            focusField(first.el ?? $(deviceFieldFor[first.key] ?? ''));
            if (!created) files = null; // the name may change, re-ask next time
            return;
          }
          generateTxt = choice === 'anyway';
          setBusy(t('nd.busy.check'));
        }

        if (existing) {
          // Always re-submitted, never gated on `created` — GLPI is
          // authoritative and the technician is explicitly allowed to keep
          // editing these fields after the first update, unlike the normal
          // create flow below.
          setBusy(t('nd.busy.update'));
          const result = await UpdateFullComputer(existing.id, input);
          created = result.computer;
          createWarnings = result.warnings ?? [];
        } else if (!created) {
          setBusy(t('nd.busy.create'));
          const result = await CreateFullComputer(input);
          created = result.computer;
          createWarnings = result.warnings ?? [];
          deviceFields.disabled = true;
          btnDetect.disabled = true;
        }
        if (!created) {
          // Unreachable: every path above either already had `created` set
          // or just set it — this only convinces the type checker.
          throw new Error('internal: created is unexpectedly null');
        }

        setBusy(t('nd.busy.finish'));
        const documents = await toAttachments(files, created.name);
        if (generateTxt) {
          documents.push(specSheetAttachment(await GenerateSpecSheetTXT(specForm.read(specBase(created))), created.name));
        }
        // A string, not a number — GLPI's own field is free-text (real
        // values include decimals like "90.7"), and sending a JSON number
        // there was silently rejected. val() already trims to ''/a plain
        // numeric string, which is exactly what GLPI itself stores.
        const akku = (i: number) => val(`f-akku${i}`) || undefined;
        await FinishDevice(
          main.FinishDeviceInput.createFrom({
            computerID: created.id,
            zusatzdaten: {
              mngelfield: val('f-maengel') || undefined,
              akkugesundheitinfield: akku(1),
              akkugesundheittwoinfield: akku(2),
              betriebssystemaktiviertfield: val('f-osaktiv') === '1',
            },
            documents: documents.map((a) => ({
              filename: a.filename,
              sourcePath: a.sourcePath ?? '',
              content: a.content,
              documentCategoryID: a.documentCategoryID,
            })),
          })
        );
        renderDone(created, createWarnings, generateTxt || hasSpecSheet(files));
      } catch (err) {
        console.error(err);
        createError.textContent = created ? t('nd.err.finish') : t('nd.err.create');
        setBusy(null);
      }
    });
  }

  function renderDone(computer: glpi.Computer, warnings: string[], withTxt: boolean) {
    content.innerHTML = `
      <div class="intake-screen">
        <section class="card">
          <h2 class="card-title">${escapeHtml(t('nd.done.title', {name: computer.name}))}</h2>
          <p class="muted">${t(withTxt ? 'nd.done.withTxt' : 'nd.done.withoutTxt')}</p>
          ${
            warnings.length
              ? `<p class="muted spec-gap">${t('nd.done.warnings')}</p>${warnings
                  .map((w) => `<p class="warning">${escapeHtml(w)}</p>`)
                  .join('')}`
              : ''
          }
        </section>
        <button class="submit-btn" id="btn-next">${t('nd.next')}</button>
      </div>
    `;
    content.querySelector('#btn-next')!.addEventListener('click', () => renderNewDevice(content, asUser));
  }
}
