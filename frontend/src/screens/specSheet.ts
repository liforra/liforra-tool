// The "Preisschild-TXT" (pc_info<name>.txt) section of the review screen:
// defaults from what's already known, the form, and the completeness check
// behind "Informationen für TXT Datei Fehlen". Formats and defaults follow
// the 458 spec sheets already in GLPI (checked 2026-09-18), e.g.
// "256 GB M.2 SATA SSD", "8 GB DDR4", "2133 MHz", "2.50 GHz".
import {specsheet} from '../../wailsjs/go/models';
import type {glpi, hwinfo, main, toolstar} from '../../wailsjs/go/models';
import {escapeHtml} from '../html';
import {t, type Key} from '../i18n';

export interface SpecSheetSource {
  computer: glpi.Computer;
  // The raw hardware scan — always present on "Neues Gerät" (with input:
  // the values as the technician confirmed them); review.ts also passes
  // just {info} for an existing device, since this app runs on whatever
  // device it's finishing, so info.disks[].serial etc. are usable there
  // too even without a NewDeviceInput. Kept even though nothing reads
  // info.disks[].serial yet — ready for the drive-serial TXT field once
  // its exact format is decided (see project chat, 2026-09-29).
  scan?: {info: hwinfo.Info; input?: main.NewDeviceInput};
  // A toolstar test report found on the USB stick, for existing devices.
  testReport?: toolstar.Report;
}

const trademarkRe = /\((r|tm|c)\)|[®™©]/gi;

function cleanName(s: string): string {
  return s.replace(trademarkRe, '').replace(/\s+/g, ' ').trim();
}

// "11th Gen Intel(R) Core(TM) i5-1135G7 @ 2.40GHz" -> "Intel Core i5-1135G7"
// Exported: the New Device form's top-level CPU card uses this too, to
// split the raw scanned string into a GLPI-matchable model name plus a
// separate speed field (see cpuSpeed below) — GLPI's own DeviceProcessor
// catalog doesn't care about clock speed, only the TXT spec sheet does.
export function cleanCpu(s: string): string {
  return cleanName(s.replace(/@.*$/, '').replace(/\bCPU\b/gi, '').replace(/^\s*\d+(st|nd|rd|th)\s+Gen\s+/i, ''));
}

// Base clock from the name ("... @ 2.50GHz"); WMI's max clock is only a
// fallback since it may report turbo speed instead.
export function cpuSpeed(name: string, maxClockMHz?: number): string {
  const m = /@\s*([\d.]+)\s*GHz/i.exec(name);
  if (m) return `${Number(m[1]).toFixed(2)} GHz`;
  return maxClockMHz ? `${(maxClockMHz / 1000).toFixed(2)} GHz` : '';
}

function capacity(gb: number): string {
  return gb >= 1000 ? `${Math.round(gb / 1000)} TB` : `${Math.round(gb)} GB`;
}

function isLaptopType(type: string): boolean {
  return /laptop|notebook|ultrabook|convertible|tablet/i.test(type);
}

const produktarten: [RegExp, string][] = [
  [/laptop|notebook|ultrabook|convertible/i, 'Laptop'],
  [/tablet/i, 'Tablet'],
  [/sff|small form/i, 'SFF'],
  [/mini/i, 'Mini-PC'],
  [/all-in-one/i, 'All-in-One-PC'],
  [/workstation/i, 'Workstation'],
  [/tower|desktop/i, 'Desktop PC'],
];

function produktart(type: string): string {
  return produktarten.find(([re]) => re.test(type))?.[1] ?? type;
}

// Spec sheets name the model without the brand ("Latitude 5480"), GLPI
// models usually include it ("Dell Latitude 5480").
function stripBrand(model: string, brand: string): string {
  const m = model.trim();
  return brand && m.toLowerCase().startsWith(brand.toLowerCase() + ' ') ? m.slice(brand.length + 1) : m;
}

// A discrete GPU is the one signal available for "Gaming"/"Grafik- und
// Videodesign" — Windows doesn't expose a VRAM figure worth trusting, and an
// integrated chip's name always says so.
function isDiscreteGpu(name: string): boolean {
  if (!name) return false;
  if (/intel/i.test(name) && !/\barc\b/i.test(name)) return false; // Intel UHD/HD/Iris — integrated
  if (/radeon(\(tm\))?\s*(vega\s*\d*\s*)?graphics\b/i.test(name)) return false; // AMD APU — integrated
  return /nvidia|geforce|quadro|\brtx\b|\bgtx\b|radeon\s*(rx|pro)|\barc\b/i.test(name);
}

export function specDefaults({computer, scan, testReport}: SpecSheetSource): specsheet.Fields {
  const input = scan?.input;
  const info = scan?.info;
  const type = computer.type?.name || input?.type || '';
  const brand = computer.manufacturer?.name || input?.manufacturer || '';
  const cpuRaw = input?.cpuModel || testReport?.cpuModel || '';
  const osName = (input?.osName || '').replace(/^Microsoft\s+/i, '');

  const memory = input?.memory ?? [];
  const ramGB = memory.length
    ? memory.reduce((sum, m) => sum + (m.capacityGB || 0), 0)
    : (testReport?.ramTotalMB ?? 0) / 1024;
  const ddrType = memory[0]?.ddrType || testReport?.ramModules?.[0]?.ddrType || '';
  const ramSpeed = memory[0]?.speedMHz || testReport?.ramModules?.[0]?.speedMHz || 0;

  const disks = input?.disks?.length
    ? input.disks.map((d) => `${capacity(d.capacityGB)} ${d.kind}`.trim())
    : (testReport?.disks ?? []).map((d) => `${capacity(d.capacityGB)} ${[d.interface, d.isSSD ? 'SSD' : 'HDD'].filter(Boolean).join(' ')}`);

  // A guess from the specs, same as the rest of this form's defaults — the
  // technician can still change every box. Büroarbeit/Multimedia stay true
  // regardless, matching what nearly every existing sheet says.
  const discreteGpu = isDiscreteGpu(input?.gpuModel || testReport?.gpuModel || '');
  const wellSpecced = ramGB >= 16 && (info?.cpuCores ?? 0) >= 4;

  return specsheet.Fields.createFrom({
    sku: computer.name,
    geraetetyp: type ? (isLaptopType(type) ? 'Laptop' : 'Desktop') : '',
    marke: brand,
    modell: stripBrand(computer.model?.name || input?.model || '', brand),
    produktart: produktart(type),
    mainboard: info?.mainboard ?? '',
    prozessor: cleanCpu(cpuRaw),
    prozessorgeschwindigkeit: cpuRaw ? cpuSpeed(cpuRaw, info?.cpuMaxClockMHz) : '',
    ram: ramGB ? `${Math.round(ramGB)} GB ${ddrType}`.trim() : '',
    ramGeschwindigkeit: ramSpeed ? `${ramSpeed} MHz` : '',
    ssd: disks[0] ?? '',
    sekundaerspeicher: disks[1] ?? (disks.length ? 'Nicht vorhanden' : ''),
    grafikeinheit: cleanName(input?.gpuModel || testReport?.gpuModel || ''),
    betriebssystem: osName,
    // From the BIOS's own release date — not authoritative if the BIOS has
    // since been updated, but there's no better source; the technician
    // adjusts if it's off.
    erscheinungsjahr: info?.releaseYear ? String(info.releaseYear) : '',
    farbe: '',
    wlan: info?.wlan ?? false,
    bluetooth: info?.bluetooth ?? false,
    laufwerk: info ? (info.opticalDrive ? `${info.opticalDrive}-Laufwerk` : 'Nicht vorhanden') : '',
    sdKartensteckplatz: info?.sdCardReader ?? false,
    // Ethernet is the one port count Windows can actually report (a
    // physical wired NIC); the rest have no such signal — see
    // hwinfo.Info.EthernetPorts.
    ports: {
      vga: 0,
      displayPort: 0,
      hdmi: 0,
      dvi: 0,
      usb3: 0,
      usb2: 0,
      ethernet: info?.ethernetPorts ?? 0,
      kopfhoerer: 0,
      mikrofon: 0,
      lineIn: 0,
      ps2: 0,
    },
    gewicht: '',
    abmessungen: '',
    anwendungsgebiete: {
      bueroarbeit: true,
      multimedia: true,
      gaming: discreteGpu,
      programmierung: wellSpecced,
      grafikDesign: discreteGpu && wellSpecced,
    },
    lieferumfang: {
      pcSystem: true,
      kabel: true,
      windows11Digital: !!info?.osActivated && /windows 11/i.test(osName),
      windows11Key: false,
    },
    bemerkungen: '',
  });
}

type TextKey =
  | 'sku' | 'produktart' | 'marke' | 'modell' | 'mainboard' | 'erscheinungsjahr' | 'farbe'
  | 'prozessor' | 'prozessorgeschwindigkeit' | 'ram' | 'ramGeschwindigkeit' | 'ssd' | 'sekundaerspeicher'
  | 'grafikeinheit' | 'betriebssystem' | 'laufwerk' | 'gewicht' | 'abmessungen';

export type RequiredKey = TextKey | 'geraetetyp' | 'ports';

// Filled in on at least 95% of the existing sheets — anything else is
// optional and never triggers the "missing" prompt.
const requiredFields: [TextKey | 'geraetetyp', Key][] = [
  ['sku', 'ss.sku'],
  ['geraetetyp', 'ss.deviceType'],
  ['marke', 'ss.brand'],
  ['modell', 'ss.model'],
  ['produktart', 'ss.productType'],
  ['mainboard', 'ss.mainboard'],
  ['prozessor', 'ss.cpu'],
  ['prozessorgeschwindigkeit', 'ss.cpuSpeed'],
  ['ram', 'ss.ramShort'],
  ['ramGeschwindigkeit', 'ss.ramSpeed'],
  ['ssd', 'ss.storage'],
  ['grafikeinheit', 'ss.gpu'],
  ['betriebssystem', 'ss.os'],
  ['erscheinungsjahr', 'ss.year'],
  ['farbe', 'ss.color'],
  ['gewicht', 'ss.weight'],
  ['abmessungen', 'ss.dimensionsShort'],
];

const textKeys: TextKey[] = [
  'sku', 'produktart', 'marke', 'modell', 'mainboard', 'erscheinungsjahr', 'farbe', 'prozessor', 'prozessorgeschwindigkeit',
  'ram', 'ramGeschwindigkeit', 'ssd', 'sekundaerspeicher', 'grafikeinheit', 'betriebssystem', 'laufwerk', 'gewicht', 'abmessungen',
];

// A label is either a translation key or a name that reads the same in every
// language (connector names).
type Label = {key: Key} | {plain: string};

const portFields: [keyof specsheet.Ports, Label][] = [
  ['usb3', {plain: 'USB 3.0 (Typ-A)'}],
  ['usb2', {plain: 'USB 2.0 (Typ-A)'}],
  ['hdmi', {plain: 'HDMI'}],
  ['displayPort', {plain: 'DisplayPort'}],
  ['vga', {plain: 'VGA'}],
  ['dvi', {plain: 'DVI'}],
  ['ethernet', {plain: 'Ethernet (RJ-45)'}],
  ['kopfhoerer', {key: 'ss.port.headphones'}],
  ['mikrofon', {key: 'ss.port.microphone'}],
  ['lineIn', {plain: 'Line-In'}],
  ['ps2', {plain: 'PS/2'}],
];

const anwendungsgebietFields: [keyof specsheet.Anwendungsgebiete, Label][] = [
  ['bueroarbeit', {key: 'ss.use.office'}],
  ['multimedia', {key: 'ss.use.multimedia'}],
  ['programmierung', {key: 'ss.use.programming'}],
  ['gaming', {key: 'ss.use.gaming'}],
  ['grafikDesign', {key: 'ss.use.graphics'}],
];

const lieferumfangFields: [keyof specsheet.Lieferumfang, Label][] = [
  ['pcSystem', {key: 'ss.del.pc'}],
  ['kabel', {key: 'ss.del.cable'}],
  ['windows11Digital', {key: 'ss.del.win11Digital'}],
  ['windows11Key', {key: 'ss.del.win11Key'}],
];

function labelText(l: Label): string {
  return 'key' in l ? t(l.key) : l.plain;
}

function text(key: TextKey, label: string, value: string, extra = ''): string {
  return `
    <div class="field">
      <label for="s-${key}">${label}</label>
      <input id="s-${key}" type="text" value="${escapeHtml(value)}" ${extra} />
    </div>
  `;
}

function toggle(id: string, label: string, checked: boolean): string {
  return `
    <label class="toggle-row" for="${id}">
      <span class="toggle-title">${label}</span>
      <input type="checkbox" class="switch" id="${id}" ${checked ? 'checked' : ''} />
    </label>
  `;
}

export interface MissingSpecField {
  key: RequiredKey;
  label: string;
  // The input to jump to, or null when the value comes from outside this
  // form (the device fields on "Neues Gerät").
  el: HTMLElement | null;
}

// Whether a Lenovo spec-sheet lookup is worth attempting for this brand —
// see internal/lenovospecs; the only vendor with a confirmed public source.
export function isLenovo(brand: string): boolean {
  return /^lenovo$/i.test(brand.trim());
}

export interface SpecSheetForm {
  // The form's inputs over base — base supplies every field the form
  // doesn't show (defaults to the values it was rendered with).
  read(base?: specsheet.Fields): specsheet.Fields;
  missing(base?: specsheet.Fields): MissingSpecField[];
  // Fills text fields that are still empty — for data that arrives after
  // rendering, without overwriting what the technician typed.
  fillEmpty(fields: specsheet.Fields): void;
}

// withCore: whether to show the fields that describe the hardware itself
// (SKU, type, brand, model, CPU, RAM, storage, GPU, OS). "Neues Gerät"
// already has all of those in its device fields, so it only shows the rest.
export function renderSpecSheetForm(container: HTMLElement, d: specsheet.Fields, withCore = true): SpecSheetForm {
  container.innerHTML = `
    <h3 class="spec-subtitle">${t('ss.general')}</h3>
    ${
      withCore
        ? `
    <div class="grid-3">
      ${text('sku', 'SKU', d.sku)}
      <div class="field">
        <label for="s-geraetetyp">${t('ss.deviceType')}</label>
        <select id="s-geraetetyp">
          ${['', 'Desktop', 'Laptop'].map((v) => `<option value="${v}"${v === d.geraetetyp ? ' selected' : ''}>${v || '–'}</option>`).join('')}
        </select>
      </div>
      ${text('produktart', t('ss.productType'), d.produktart)}
    </div>
    <div class="grid-2">
      ${text('marke', t('ss.brand'), d.marke)}
      ${text('modell', t('ss.model'), d.modell)}
    </div>`
        : ''
    }
    <div class="grid-2">
      ${text('mainboard', t('ss.mainboard'), d.mainboard)}
      ${text('erscheinungsjahr', t('ss.year'), d.erscheinungsjahr, `inputmode="numeric" placeholder="${t('ss.eg', {value: '2017'})}"`)}
    </div>
    <div class="grid-2">
      ${text('farbe', t('ss.color'), d.farbe, 'list="s-farben"')}
      <datalist id="s-farben"><option value="Schwarz"></option><option value="Silber"></option><option value="Grau"></option><option value="Weiß"></option></datalist>
      ${text('laufwerk', t('ss.drive'), d.laufwerk)}
    </div>

    ${
      withCore
        ? `
    <h3 class="spec-subtitle">${t('ss.system')}</h3>
    <div class="grid-2">
      ${text('prozessor', t('ss.cpu'), d.prozessor)}
      ${text('prozessorgeschwindigkeit', t('ss.cpuSpeed'), d.prozessorgeschwindigkeit)}
    </div>
    <div class="grid-2">
      ${text('ram', t('ss.ram'), d.ram)}
      ${text('ramGeschwindigkeit', t('ss.ramSpeed'), d.ramGeschwindigkeit)}
    </div>
    <div class="grid-2">
      ${text('ssd', t('ss.storage'), d.ssd)}
      ${text('sekundaerspeicher', t('ss.secondary'), d.sekundaerspeicher)}
    </div>
    <div class="grid-2">
      ${text('grafikeinheit', t('ss.gpu'), d.grafikeinheit)}
      ${text('betriebssystem', t('ss.os'), d.betriebssystem)}
    </div>`
        : ''
    }

    <h3 class="spec-subtitle">${t('ss.connectivity')}</h3>
    <div class="toggle-grid">
      ${toggle('s-wlan', 'WLAN', d.wlan)}
      ${toggle('s-bluetooth', 'Bluetooth', d.bluetooth)}
      ${toggle('s-sd', t('ss.sdSlot'), d.sdKartensteckplatz)}
    </div>

    <h3 class="spec-subtitle">${t('ss.ports')}</h3>
    <div class="port-grid">
      ${portFields
        .map(
          ([key, label]) => `
        <div class="field">
          <label for="s-port-${key}">${labelText(label)}</label>
          <input id="s-port-${key}" type="number" min="0" placeholder="0" value="${d.ports[key] || ''}" />
        </div>`
        )
        .join('')}
    </div>

    <h3 class="spec-subtitle">${t('ss.weightSize')}</h3>
    <div class="grid-2">
      ${text('gewicht', t('ss.weight'), d.gewicht, `placeholder="${t('ss.eg', {value: '1,3 kg'})}"`)}
      ${text('abmessungen', t('ss.dimensions'), d.abmessungen, `placeholder="${t('ss.eg', {value: '339 x 24 x 235 mm'})}"`)}
    </div>

    <h3 class="spec-subtitle">${t('ss.usage')}</h3>
    <div class="toggle-grid">
      ${anwendungsgebietFields.map(([key, label]) => toggle(`s-anw-${key}`, labelText(label), d.anwendungsgebiete[key])).join('')}
    </div>

    <h3 class="spec-subtitle">${t('ss.delivery')}</h3>
    <div class="toggle-grid">
      ${lieferumfangFields.map(([key, label]) => toggle(`s-lief-${key}`, labelText(label), d.lieferumfang[key])).join('')}
    </div>

    <div class="field spec-gap">
      <label for="s-bemerkungen">${t('ss.remarks')}</label>
      <textarea id="s-bemerkungen" rows="2">${escapeHtml(d.bemerkungen)}</textarea>
    </div>
  `;

  const find = <T extends HTMLElement>(id: string) => container.querySelector<T>(`#${id}`);
  const val = (id: string) => find<HTMLInputElement>(id)!.value.trim();
  const checked = (id: string) => find<HTMLInputElement>(id)!.checked;

  function read(base: specsheet.Fields = d): specsheet.Fields {
    const shownText = Object.fromEntries(textKeys.filter((k) => find(`s-${k}`)).map((k) => [k, val(`s-${k}`)]));
    const typeSelect = find<HTMLSelectElement>('s-geraetetyp');
    return specsheet.Fields.createFrom({
      ...base,
      ...shownText,
      geraetetyp: typeSelect ? typeSelect.value : base.geraetetyp,
      wlan: checked('s-wlan'),
      bluetooth: checked('s-bluetooth'),
      sdKartensteckplatz: checked('s-sd'),
      ports: Object.fromEntries(portFields.map(([k]) => [k, Number(val(`s-port-${k}`)) || 0])),
      anwendungsgebiete: Object.fromEntries(anwendungsgebietFields.map(([k]) => [k, checked(`s-anw-${k}`)])),
      lieferumfang: Object.fromEntries(lieferumfangFields.map(([k]) => [k, checked(`s-lief-${k}`)])),
      bemerkungen: find<HTMLTextAreaElement>('s-bemerkungen')!.value.trim(),
    });
  }

  function missing(base?: specsheet.Fields): MissingSpecField[] {
    const fields = read(base);
    const result: MissingSpecField[] = requiredFields
      .filter(([key]) => !String(fields[key] ?? '').trim())
      .map(([key, label]) => ({key, label: t(label), el: find<HTMLElement>(`s-${key}`)}));
    if (!portFields.some(([k]) => fields.ports[k] > 0)) {
      result.push({key: 'ports', label: t('ss.portsMissing'), el: find<HTMLElement>(`s-port-${portFields[0][0]}`)});
    }
    return result;
  }

  function fillEmpty(fields: specsheet.Fields) {
    for (const k of textKeys) {
      const input = find<HTMLInputElement>(`s-${k}`);
      const value = String(fields[k] ?? '');
      if (input && !input.value.trim() && value) input.value = value;
    }
    for (const [key] of portFields) {
      const input = find<HTMLInputElement>(`s-port-${key}`);
      const value = fields.ports?.[key] ?? 0;
      if (input && !Number(input.value) && value > 0) input.value = String(value);
    }
  }

  return {read, missing, fillEmpty};
}
