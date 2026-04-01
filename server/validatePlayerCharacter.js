/**
 * Player character post-processing for /generate-character.
 *
 * Policy (avoid endless LLM workarounds):
 * - `normalizeGeneratedPlayerCharacter` only enforces **stable JSON shapes** (arrays, string coercion,
 *   JSON-string weapons, damage → string). If the model sends a mistaken `tools` array, those lines are
 *   appended into `equipment` and `tools` is removed (tools are not a separate category).
 *   It adds default `coinage` (D&D 5e coins) if missing and prepends clothing when needed
 *   and equipment lacks clothing-like text (language-aware when `opts.language` hints Spanish).
 *   It strips `equipment` lines that duplicate a `weapons[].name` (same item must not appear in both).
 *   It does **not** invent weapons or read other alternate field names.
 * - `validateGeneratedPlayerCharacter` checks **identity + defense + array types**; weapon rows are
 *   validated only when present. `languages` must be a **non-empty** string array (PHB-derived).
 *   Empty `equipment` / `weapons` is allowed; the sheet UI and combat prompts already handle missing gear.
 */

function normalizeGearText(s) {
  let t = String(s || '')
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '');
  t = t.replace(/\s*\(\d+\)\s*$/, '').trim();
  t = t.replace(/^\d+\s*[x×]\s*/i, '').trim();
  return t;
}

/** Base weapon name for matching (strip leading "2×", lowercase, accents). */
function weaponRowNormalizedName(row) {
  if (!row || typeof row !== 'object') return '';
  let raw = String(row.name || '').trim();
  raw = raw.replace(/^\d+\s*[x×]\s*/i, '').trim();
  return normalizeGearText(raw);
}

/** Drop equipment lines that duplicate a `weapons[].name` (weapons belong only in `weapons`). */
function dedupeEquipmentAgainstWeapons(equipment, weapons) {
  if (!Array.isArray(equipment) || !Array.isArray(weapons)) return equipment;
  const names = new Set();
  for (const w of weapons) {
    const b = weaponRowNormalizedName(w);
    if (b) names.add(b);
  }
  if (!names.size) return equipment;
  const out = [];
  for (const line of equipment) {
    const key = normalizeGearText(line);
    if (names.has(key)) {
      // eslint-disable-next-line no-console
      console.warn(
        'dedupeEquipmentAgainstWeapons: removed equipment line that duplicates weapons[].name:',
        String(line).slice(0, 120)
      );
      continue;
    }
    out.push(line);
  }
  return out;
}

const DAMAGE_HAS_DICE = /\d*d\d+/i;

/**
 * True if an equipment line describes **base garments** (shirt, tunic, dress, etc.).
 * Cloaks, capes, boots-only, belts, hats, etc. do **not** count — a light cloak is not a full outfit.
 */
const BASE_GARMENT_RE =
  /common\s+clothes|travel\s+clothes|everyday\s+clothes|street\s+clothes|underclothes|\bclothing\b|\bshirt\b|\bblouse\b|\btrousers\b|\bpants\b|\bbreeches\b|\bhose\b|\bdoublet\b|\btunic\b|\btúnica\b|\btunica\b|\brobes?\b|\bdress\b|\bgarb\b|\bvestments\b|\bhabit\b|\bcamisa\b|\bpantalones\b|\bvestido\b|\bsayo\b|\bcalzones\b|\bropa\s+de\s+viaje\b|\bropa\s+corriente\b|\bropa\s+interior\b|\bindumentaria\s+corriente\b|\bropa\s+y\s+calzado\b/i;

/** True if armor list includes a body armor piece (not shield-only). */
function hasBodyArmorLines(armorLines) {
  if (!Array.isArray(armorLines) || !armorLines.length) return false;
  const shieldOnly = /^(escudo|shield)\b/i;
  const bodyArmor =
    /(leather|studded|hide|padded|chain|ring|scale|breastplate|half[\s-]?plate|splint|plate|cuero|acolchada|anillos?|malla|cota|escamas|peto|media\s*placa|placas|cuero\s*reforzado)/i;
  return armorLines.some((line) => {
    const s = String(line || '').trim();
    if (!s) return false;
    if (shieldOnly.test(s)) return false;
    return bodyArmor.test(s);
  });
}

function equipmentHasBaseGarments(equipmentLines) {
  if (!Array.isArray(equipmentLines)) return false;
  return equipmentLines.some((line) => BASE_GARMENT_RE.test(String(line || '')));
}

function defaultClothesLine(language) {
  const lang = String(language || '').toLowerCase();
  if (lang.startsWith('span')) {
    return 'Ropa de viaje y calzado (sin armadura)';
  }
  return 'Travel clothes and boots (no armor)';
}

/** Minimal placeholder when older saves omit `languages` (PHB: almost all PCs know Common). */
function defaultLanguagesFallback(language) {
  const lang = String(language || '').toLowerCase();
  if (lang.startsWith('span')) {
    return ['Común'];
  }
  return ['Common'];
}

/** D&D 5e coin types (Player’s Handbook). Order used for display / merge. */
const CURRENCY_KEYS = ['pp', 'gp', 'ep', 'sp', 'cp'];

/**
 * Non-negative integer coin count (single denomination).
 * @returns {number|null}
 */
function normalizeCoinAmount(raw) {
  if (raw == null || raw === '') return null;
  const s = String(raw).trim().replace(/^\+/, '');
  const n = Number(s.replace(/[^\d.-]/g, ''));
  if (!Number.isFinite(n) || n < 0) return null;
  return Math.min(9999999, Math.floor(n));
}

/**
 * Full 5e coin bag; missing keys become 0.
 * @param {object|null|undefined} raw
 * @returns {{ pp: number, gp: number, ep: number, sp: number, cp: number }}
 */
function normalizeCoinageObject(raw) {
  const out = { pp: 0, gp: 0, ep: 0, sp: 0, cp: 0 };
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return out;
  for (const k of CURRENCY_KEYS) {
    const v = normalizeCoinAmount(raw[k]);
    if (v != null) out[k] = v;
  }
  return out;
}

/**
 * Patch existing coinage with envelope values (only defined keys on `patch` overwrite).
 * @param {object|null|undefined} existing
 * @param {object|null|undefined} patch
 */
function mergeCoinage(existing, patch) {
  const base = normalizeCoinageObject(existing);
  if (!patch || typeof patch !== 'object' || Array.isArray(patch)) return base;
  const out = { ...base };
  for (const k of CURRENCY_KEYS) {
    if (patch[k] != null) {
      const v = normalizeCoinAmount(patch[k]);
      if (v != null) out[k] = v;
    }
  }
  return out;
}

/** Mutates `pc`: normalizes `coinage`, folds mistaken `currency` key from models, drops `currency`. */
function applyCoinageToPlayerCharacterInPlace(pc) {
  if (!pc || typeof pc !== 'object') return;
  if (pc.coinage != null && typeof pc.coinage === 'object' && !Array.isArray(pc.coinage)) {
    pc.coinage = normalizeCoinageObject(pc.coinage);
  } else if (pc.currency != null && typeof pc.currency === 'object' && !Array.isArray(pc.currency)) {
    pc.coinage = normalizeCoinageObject(pc.currency);
    // eslint-disable-next-line no-console
    console.warn(
      'applyCoinageToPlayerCharacterInPlace: model sent `currency`; normalized onto `coinage`. Prompt requires key `coinage` only.'
    );
  } else {
    pc.coinage = { pp: 0, gp: 15, ep: 0, sp: 0, cp: 0 };
    // eslint-disable-next-line no-console
    console.warn('applyCoinageToPlayerCharacterInPlace: missing coinage; using default 15 gp.');
  }
  delete pc.currency;
}

/** Readable damage string for validation (handles a few object shapes models use). */
function damageToString(raw) {
  if (raw == null) return '';
  if (typeof raw === 'string') return raw.trim();
  if (typeof raw === 'number' && Number.isFinite(raw)) return String(raw);
  if (typeof raw === 'object') {
    if (raw.dice != null) return String(raw.dice).trim();
    if (raw.formula != null) return String(raw.formula).trim();
    if (raw.count != null && raw.sides != null) {
      const c = Number(raw.count);
      const s = Number(raw.sides);
      if (Number.isFinite(c) && Number.isFinite(s)) return `${c}d${s}`;
    }
  }
  return String(raw).trim();
}

function compactDamage(s) {
  let t = damageToString(s);
  try {
    t = t.normalize('NFKC');
  } catch (e) {
    /* ignore */
  }
  return t.replace(/\s+/g, '');
}

/** String list: null → []; string → [s]; object → values; array → string items. */
function asStringArray(val) {
  if (val == null) return [];
  if (Array.isArray(val)) return val.map((x) => String(x).trim()).filter(Boolean);
  if (typeof val === 'object') {
    return Object.values(val)
      .map((x) => String(x).trim())
      .filter(Boolean);
  }
  const s = String(val).trim();
  if (!s || /^none$/i.test(s) || s === '—' || s === '-') return [];
  return [s];
}

function asWeaponsArray(val) {
  let w = val;
  if (w == null) return [];
  if (typeof w === 'string') {
    try {
      w = JSON.parse(w);
    } catch (e) {
      return [];
    }
  }
  if (w && typeof w === 'object' && !Array.isArray(w)) w = [w];
  if (!Array.isArray(w)) return [];
  return w
    .filter((row) => row != null && typeof row === 'object')
    .map((row) => {
      const r = { ...row };
      r.name = String(r.name || '').trim();
      r.damage = damageToString(r.damage);
      if (r.attack_bonus !== undefined && r.attack_bonus !== null && r.attack_bonus !== '') {
        const n = Number(String(r.attack_bonus).trim().replace(/^\+/, ''));
        if (Number.isFinite(n)) r.attack_bonus = n;
      }
      return r;
    });
}

/**
 * Stable shapes only — no synthetic weapons; may add clothes line and default gold.
 * @param {object} pc
 * @param {{ language?: string }} [opts]
 * @returns {object}
 */
function normalizeGeneratedPlayerCharacter(pc, opts) {
  if (!pc || typeof pc !== 'object') return pc;
  const language = opts && opts.language;
  const out = { ...pc };
  out.armor = asStringArray(out.armor);
  out.equipment = asStringArray(out.equipment);
  out.weapons = asWeaponsArray(out.weapons);
  out.equipment = dedupeEquipmentAgainstWeapons(out.equipment, out.weapons);
  if (out.languages == null) {
    out.languages = [];
  } else if (!Array.isArray(out.languages)) {
    out.languages = [String(out.languages)].filter(Boolean);
  } else {
    out.languages = out.languages.map((x) => String(x).trim()).filter(Boolean);
  }

  applyCoinageToPlayerCharacterInPlace(out);

  if (!hasBodyArmorLines(out.armor) && !equipmentHasBaseGarments(out.equipment)) {
    out.equipment = [defaultClothesLine(language), ...out.equipment];
  }

  // Separate `tools` array: merge into equipment (schema uses equipment only).
  if (out.tools != null) {
    const toolLines = asStringArray(out.tools);
    if (toolLines.length) {
      const seen = new Set(out.equipment.map((s) => s.toLowerCase()));
      for (const line of toolLines) {
        if (!seen.has(line.toLowerCase())) {
          seen.add(line.toLowerCase());
          out.equipment.push(line);
        }
      }
      // eslint-disable-next-line no-console
      console.warn(
        'normalizeGeneratedPlayerCharacter: merged playerCharacter.tools into equipment; do not emit a tools key.'
      );
    }
    delete out.tools;
  }
  return out;
}

/**
 * Idempotent defaults for saved games: starting gold and a real garments line when unarmored.
 * Use on load/persist so older sheets and models that omit fields still get a correct display.
 * @param {object} pc
 * @param {{ language?: string }} [opts]
 * @returns {object}
 */
function ensurePlayerCharacterSheetDefaults(pc, opts) {
  if (!pc || typeof pc !== 'object') return pc;
  const language = opts && opts.language;
  const out = JSON.parse(JSON.stringify(pc));
  out.armor = asStringArray(out.armor);
  out.equipment = asStringArray(out.equipment);
  out.equipment = dedupeEquipmentAgainstWeapons(out.equipment, asWeaponsArray(out.weapons));

  applyCoinageToPlayerCharacterInPlace(out);

  if (!hasBodyArmorLines(out.armor) && !equipmentHasBaseGarments(out.equipment)) {
    out.equipment = [defaultClothesLine(language), ...out.equipment];
  }

  if (out.languages == null) {
    out.languages = [];
  } else if (!Array.isArray(out.languages)) {
    out.languages = [String(out.languages)].filter(Boolean);
  } else {
    out.languages = out.languages.map((x) => String(x).trim()).filter(Boolean);
  }

  if (!out.languages.length) {
    out.languages = defaultLanguagesFallback(language);
    // eslint-disable-next-line no-console
    console.warn(
      'ensurePlayerCharacterSheetDefaults: missing languages; applied Common-only placeholder — regenerate character for full PHB list.'
    );
  }
  return out;
}

/**
 * @returns {{ ok: true } | { ok: false, error: string }}
 */
function validateGeneratedPlayerCharacter(pc) {
  if (!pc || typeof pc !== 'object') return { ok: false, error: 'playerCharacter must be an object' };

  if (!String(pc.name || '').trim()) {
    return { ok: false, error: 'playerCharacter.name is required' };
  }

  const hp = Number(pc.max_hp);
  const ac = Number(pc.ac);
  if (!Number.isFinite(hp)) {
    return { ok: false, error: 'playerCharacter.max_hp must be a finite number' };
  }
  if (!Number.isFinite(ac)) {
    return { ok: false, error: 'playerCharacter.ac must be a finite number' };
  }

  if (!Array.isArray(pc.armor)) {
    return { ok: false, error: 'playerCharacter.armor must be an array (use [] if none).' };
  }
  if (!Array.isArray(pc.equipment)) {
    return { ok: false, error: 'playerCharacter.equipment must be an array (may be []).' };
  }
  if (!Array.isArray(pc.weapons)) {
    return { ok: false, error: 'playerCharacter.weapons must be an array (may be []).' };
  }

  const cur = pc.coinage;
  if (!cur || typeof cur !== 'object' || Array.isArray(cur)) {
    return { ok: false, error: 'playerCharacter.coinage must be an object { pp, gp, ep, sp, cp } (D&D 5e).' };
  }
  for (const k of CURRENCY_KEYS) {
    const n = Number(cur[k]);
    if (!Number.isInteger(n) || n < 0) {
      return { ok: false, error: `playerCharacter.coinage.${k} must be a non-negative integer.` };
    }
  }

  if (!Array.isArray(pc.languages) || pc.languages.length === 0) {
    return {
      ok: false,
      error:
        'playerCharacter.languages must be a non-empty array of strings (D&D 5e PHB: derive from race, class, subclass, background at level 1).',
    };
  }
  for (let i = 0; i < pc.languages.length; i++) {
    if (!String(pc.languages[i] || '').trim()) {
      return { ok: false, error: `playerCharacter.languages[${i}] must be a non-empty string.` };
    }
  }

  const weapons = pc.weapons;
  for (let i = 0; i < weapons.length; i++) {
    const row = weapons[i];
    if (!row || typeof row !== 'object') return { ok: false, error: `weapons[${i}] must be an object` };
    if (!String(row.name || '').trim()) {
      return { ok: false, error: `weapons[${i}].name is required when weapons are listed` };
    }
    if (!DAMAGE_HAS_DICE.test(compactDamage(row.damage))) {
      return {
        ok: false,
        error: `weapons[${i}].damage must include dice (e.g. 1d8+2); got "${String(row.damage).slice(0, 80)}"`,
      };
    }
    const bonusRaw = row.attack_bonus == null ? '' : String(row.attack_bonus).trim().replace(/^\+/, '');
    if (bonusRaw === '' || Number.isNaN(Number(bonusRaw))) {
      return { ok: false, error: `weapons[${i}].attack_bonus must be a number` };
    }
  }

  return { ok: true };
}

module.exports = {
  validateGeneratedPlayerCharacter,
  normalizeGeneratedPlayerCharacter,
  ensurePlayerCharacterSheetDefaults,
  normalizeGearText,
  normalizeCoinAmount,
  normalizeCoinageObject,
  mergeCoinage,
  CURRENCY_KEYS,
};
