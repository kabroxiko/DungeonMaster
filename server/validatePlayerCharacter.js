/**
 * Player character post-processing for /generate-character.
 *
 * Policy (avoid endless LLM workarounds):
 * - `normalizeGeneratedPlayerCharacter` only enforces **stable JSON shapes** (arrays, string coercion,
 *   JSON-string weapons, damage → string). If the model sends a legacy `tools` array, those lines are
 *   appended into `equipment` and `tools` is removed (tools are not a separate category).
 *   It does **not** invent weapons or read other alternate field names.
 * - `validateGeneratedPlayerCharacter` checks **identity + defense + array types**; weapon rows are
 *   validated only when present. Empty `equipment` / `weapons` is allowed; the sheet UI and combat
 *   prompts already handle missing gear.
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

const DAMAGE_HAS_DICE = /\d*d\d+/i;

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
 * Stable shapes only — no synthetic items, no renaming foreign keys.
 * @param {object} pc
 * @returns {object}
 */
function normalizeGeneratedPlayerCharacter(pc) {
  if (!pc || typeof pc !== 'object') return pc;
  const out = { ...pc };
  out.armor = asStringArray(out.armor);
  out.equipment = asStringArray(out.equipment);
  out.weapons = asWeaponsArray(out.weapons);
  if (out.languages != null && !Array.isArray(out.languages)) {
    out.languages = [String(out.languages)].filter(Boolean);
  }
  // Legacy / mistaken `tools` key: same list as equipment (prompt forbids separate tools).
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

  if (pc.languages != null && !Array.isArray(pc.languages)) {
    return { ok: false, error: 'playerCharacter.languages must be an array of strings when present.' };
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

module.exports = { validateGeneratedPlayerCharacter, normalizeGeneratedPlayerCharacter, normalizeGearText };
