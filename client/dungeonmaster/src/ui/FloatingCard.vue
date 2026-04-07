<template>
  <div :class="['floating-card', { open: isOpen || embedded, 'floating-card--embedded': embedded }]">
    <div
      class="floating-panel parchment-panel ornate-border char-sheet-panel"
      v-show="embedded || isOpen"
      :role="embedded ? 'region' : 'dialog'"
      :aria-modal="embedded ? 'false' : 'true'"
      :aria-hidden="(embedded ? 'false' : (!isOpen).toString())"
      :tabindex="embedded ? undefined : -1"
      :aria-labelledby="titleId"
      :class="{ 'char-sheet-panel--embedded': embedded }"
    >
      <header class="floating-header char-sheet-header">
        <div class="char-sheet-hero">
          <h2 :id="titleId" class="ui-heading char-sheet-name">{{ character.name }}</h2>
          <p class="floating-sub char-sheet-lineage">
            <span class="char-sheet-lineage-main">{{ displayRace }}<template v-if="displaySubrace"> · {{ displaySubrace }}</template> · {{ displayClass }}</span>
            <template v-if="displaySubclass">
              <span class="char-sheet-subclass"> · {{ displaySubclass }}</span>
            </template>
            <template v-if="character.level">
              <span class="char-sheet-level"> — {{ $t('level_prefix') }}{{ character.level }}</span>
            </template>
          </p>
        </div>
      </header>
      <div class="floating-body char-sheet-body">
        <div class="char-sheet char-sheet--tabbed">
          <div class="char-sheet-tablist-wrap">
            <div
              class="char-sheet-tablist"
              role="tablist"
              :aria-label="$i18n.sheet_tabs_aria"
            >
              <button
                v-for="(tab, i) in charSheetTabs"
                :key="tab.id"
                type="button"
                role="tab"
                :id="charSheetTabDomId(i)"
                :aria-selected="charSheetActiveTab === i ? 'true' : 'false'"
                :tabindex="charSheetActiveTab === i ? 0 : -1"
                :aria-controls="charSheetPanelDomId(i)"
                class="char-sheet-tab"
                :class="{ 'char-sheet-tab--active': charSheetActiveTab === i }"
                :title="tabLabel(tab)"
                :aria-label="tabLabel(tab)"
                @click="selectCharSheetTab(i)"
              >
                <span class="char-sheet-tab__icon" aria-hidden="true">
                  <FontAwesomeIcon :icon="faCharSheetTabIcons[tab.id]" class="char-sheet-tab__fa" />
                </span>
              </button>
            </div>
          </div>

          <div class="char-sheet-tabpanels char-sheet-tabpanels--scroll">
            <div
              v-show="charSheetActiveTab === 0"
              :id="charSheetPanelDomId(0)"
              class="char-sheet-panel char-sheet-panel--character"
              role="tabpanel"
              :aria-labelledby="charSheetTabDomId(0)"
              tabindex="0"
            >
              <div class="char-sheet">
                <div
                  class="cs-vitals cs-vitals--in-attrs-tab"
                  role="group"
                  :aria-label="$i18n.sheet_aria_vitals"
                  aria-live="polite"
                >
                  <div class="cs-vitals-combat">
                    <div v-if="hpChipValue" class="cs-vital cs-vital--hp">
                      <span class="cs-vital-label">{{ $i18n.hit_points_abbr }}</span>
                      <span class="cs-vital-value">{{ hpChipValue }}</span>
                    </div>
                    <div v-if="acChipValue" class="cs-vital cs-vital--ac">
                      <span class="cs-vital-label">{{ $i18n.armor_class_abbr }}</span>
                      <span class="cs-vital-value">{{ acChipValue }}</span>
                    </div>
                  </div>
                  <div class="cs-vitals-wallet">
                    <span class="cs-wallet-heading">{{ $i18n.sheet_coinage }}</span>
                    <ul class="cs-coin-strip" :aria-label="$i18n.sheet_coinage">
                      <li
                        v-for="row in coinageStrip"
                        :key="row.key"
                        class="cs-coin-chip"
                        :class="{ 'cs-coin-chip--zero': row.amount === 0 }"
                      >
                        <span class="cs-coin-amt" aria-hidden="true">{{ row.amount }}</span>
                        <span class="cs-coin-unit">{{ row.abbr }}</span>
                      </li>
                    </ul>
                  </div>
                </div>
                <section v-if="character.stats" class="cs-block cs-block--attrs">
                  <h3 class="cs-section-title">{{ $i18n.attributes }}</h3>
                  <div class="attributes-grid char-sheet-attr-grid">
                    <div v-for="key in statKeys" :key="key" class="attr-box char-sheet-attr">
                      <div class="attr-label">{{ statAbbr(key) }}</div>
                      <div class="attr-value">{{ character.stats[key] }}</div>
                      <div class="attr-mod attr-mod-pill">{{ abilityModText(character.stats[key]) }}</div>
                    </div>
                  </div>
                </section>
              </div>
            </div>

            <div
              v-show="charSheetActiveTab === 1"
              :id="charSheetPanelDomId(1)"
              class="char-sheet-panel char-sheet-panel--bio"
              role="tabpanel"
              :aria-labelledby="charSheetTabDomId(1)"
              tabindex="0"
            >
              <div class="char-sheet">
                <section v-if="character.background" class="cs-block cs-block--phb-background">
                  <h3 class="cs-section-title">{{ $i18n.background }}</h3>
                  <p class="char-sheet-bg-role char-sheet-bg-role--in-block">{{ character.background }}</p>
                </section>

                <section v-if="character.brief_backstory" class="cs-block cs-block--story">
                  <h3 class="cs-section-title">{{ $i18n.sheet_story }}</h3>
                  <div class="cs-backstory">{{ character.brief_backstory }}</div>
                </section>

                <section v-if="languageList.length" class="cs-block cs-block--langs">
                  <h3 class="cs-section-title">{{ $i18n.sheet_languages }}</h3>
                  <div class="cs-lang-tags">
                    <span v-for="(it, i) in languageList" :key="'lang'+i" class="cs-lang-tag">{{ it }}</span>
                  </div>
                </section>
              </div>
            </div>

            <div
              v-show="charSheetActiveTab === 2"
              :id="charSheetPanelDomId(2)"
              class="char-sheet-panel char-sheet-panel--gear"
              role="tabpanel"
              :aria-labelledby="charSheetTabDomId(2)"
              tabindex="0"
            >
              <div class="char-sheet">
                <section v-if="armorRows.length" class="cs-block cs-block--armor">
                  <h3 class="cs-section-title">{{ $i18n.sheet_armor }}</h3>
                  <ul class="cs-weapon-list">
                    <li v-for="(row, i) in armorRows" :key="'a'+i" class="cs-weapon-row">
                      <div class="cs-weapon-main">
                        <span class="weapon-name">{{ row.displayName }}</span>
                      </div>
                      <div v-if="row.statsText" class="cs-weapon-stats cs-armor-stats">
                        <span class="cs-armor-ac-line">{{ row.statsText }}</span>
                      </div>
                    </li>
                  </ul>
                </section>

                <section v-if="character.weapons && character.weapons.length" class="cs-block cs-block--weapons">
                  <h3 class="cs-section-title">{{ $i18n.weapons_sheet }}</h3>
                  <ul class="cs-weapon-list">
                    <li v-for="(w, i) in character.weapons" :key="'w'+i" class="cs-weapon-row">
                      <div class="cs-weapon-main">
                        <span class="weapon-name">{{ weaponDisplayName(w) }}</span>
                        <span v-if="w.ability" class="cs-weapon-ability">{{ w.ability }}</span>
                      </div>
                      <div class="cs-weapon-stats">
                        <span v-if="w.attack_bonus != null" class="cs-weapon-hit">+{{ w.attack_bonus }}</span>
                        <span class="cs-weapon-damage">{{ weaponDamageText(w) }}</span>
                      </div>
                    </li>
                  </ul>
                </section>
                <section v-else-if="showWeaponsMissingHint" class="cs-block cs-block--weapons char-weapons-missing">
                  <h3 class="cs-section-title">{{ $i18n.weapons_sheet }}</h3>
                  <p class="weapons-missing-hint">{{ $i18n.weapons_sheet_missing }}</p>
                </section>

                <section v-if="equipmentList.length" class="cs-block cs-block--gear">
                  <h3 class="cs-section-title">{{ $i18n.equipment }}</h3>
                  <ul class="cs-list cs-list--plain">
                    <li v-for="(it, i) in equipmentList" :key="'e'+i">{{ formatEquipmentQuantity(it) }}</li>
                  </ul>
                </section>
                <section v-if="toolsList.length" class="cs-block cs-block--gear cs-block--tools">
                  <h3 class="cs-section-title">{{ $i18n.sheet_tools }}</h3>
                  <ul class="cs-list cs-list--plain">
                    <li v-for="(it, i) in toolsList" :key="'t'+i">{{ formatEquipmentQuantity(it) }}</li>
                  </ul>
                </section>
              </div>
            </div>

            <div
              v-show="charSheetActiveTab === 3"
              :id="charSheetPanelDomId(3)"
              class="char-sheet-panel char-sheet-panel--spells"
              role="tabpanel"
              :aria-labelledby="charSheetTabDomId(3)"
              tabindex="0"
            >
              <div class="char-sheet">
                <section v-if="spellsList.length" class="cs-block cs-block--spells">
                  <h3 class="cs-section-title">{{ $i18n.sheet_spells }}</h3>
                  <p v-if="spellSlotsSummary" class="cs-spell-slots-summary">{{ spellSlotsSummary }}</p>
                  <p v-if="hasLeveledSpells" class="cs-spell-rules-hint">{{ $i18n.spell_slots_explain }}</p>
                  <p
                    v-if="hasLeveledSpells && spellsExplicitPreparedMode && character.spell_prep_inferred"
                    class="cs-spell-rules-hint cs-spell-rules-hint--prep"
                  >
                    {{ $i18n.spell_overflow_known_explain }}
                  </p>
                  <p
                    v-else-if="hasLeveledSpells && spellsExplicitPreparedMode && character.spellbook_prep_enforced"
                    class="cs-spell-rules-hint cs-spell-rules-hint--prep"
                  >
                    {{ $i18n.spell_spellbook_prep_enforced_explain }}
                  </p>
                  <p
                    v-else-if="hasLeveledSpells && spellsExplicitPreparedMode"
                    class="cs-spell-rules-hint cs-spell-rules-hint--prep"
                  >
                    {{ $i18n.spell_prepared_sheet_explain }}
                  </p>
                  <div
                    v-if="spellLevelsSorted.length"
                    class="cs-spell-level-toolbar"
                    role="navigation"
                    :aria-label="$i18n.spell_level_pager_aria"
                  >
                    <template v-if="spellLevelsSorted.length > 1">
                      <button
                        type="button"
                        class="cs-spell-pager-btn"
                        :disabled="spellLevelPageIndex <= 0"
                        :aria-label="$i18n.spell_level_page_prev"
                        :title="$i18n.spell_level_page_prev"
                        @click="spellLevelPagePrev"
                      >
                        <FontAwesomeIcon :icon="faChevronLeft" class="cs-spell-pager-btn__icon" aria-hidden="true" />
                      </button>
                      <div class="cs-spell-pager-center">
                        <span class="cs-spell-pager-title">{{ spellLevelLabel(spellLevelPageCurrent) }}</span>
                        <span class="cs-spell-pager-fraction" aria-hidden="true"
                          >{{ spellLevelPageIndex + 1 }} / {{ spellLevelsSorted.length }}</span
                        >
                      </div>
                      <button
                        type="button"
                        class="cs-spell-pager-btn"
                        :disabled="spellLevelPageIndex >= spellLevelsSorted.length - 1"
                        :aria-label="$i18n.spell_level_page_next"
                        :title="$i18n.spell_level_page_next"
                        @click="spellLevelPageNext"
                      >
                        <FontAwesomeIcon :icon="faChevronRight" class="cs-spell-pager-btn__icon" aria-hidden="true" />
                      </button>
                    </template>
                    <div v-else class="cs-spell-pager-single">
                      {{ spellLevelLabel(spellLevelPageCurrent) }}
                    </div>
                  </div>
                  <ul class="cs-spell-list" :aria-label="spellListAriaLabel">
                    <li
                      v-for="(sp, i) in spellsOnCurrentLevelPage"
                      :key="'sp' + spellLevelPageCurrent + '-' + i + '-' + sp.name"
                      class="cs-spell-row"
                      :class="spellRowClassObj(sp)"
                    >
                      <span class="cs-spell-name">{{ sp.name }}</span>
                      <span class="cs-spell-meta">
                        <span
                          v-if="spellIsAtWill(sp)"
                          class="cs-spell-atwill-badge"
                          :title="$i18n.spell_atwill_title"
                        >{{ $i18n.spell_atwill_short }}</span>
                        <span
                          v-else-if="spellsExplicitPreparedMode && sp.prepared === true"
                          class="cs-spell-prepared-badge"
                          :title="$i18n.spell_prepared_title"
                        >{{ $i18n.spell_prepared_short }}</span>
                        <span
                          v-else-if="spellsExplicitPreparedMode"
                          class="cs-spell-not-prepared-badge"
                          :title="$i18n.spell_not_prepared_title"
                        >{{ $i18n.spell_not_prepared_short }}</span>
                      </span>
                    </li>
                  </ul>
                </section>
                <section v-else class="cs-block cs-block--spells cs-block--spells-empty">
                  <h3 class="cs-section-title">{{ $i18n.sheet_spells }}</h3>
                  <p class="cs-spell-empty-hint">{{ $i18n.sheet_tab_spells_empty }}</p>
                </section>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <button
      v-if="!embedded"
      type="button"
      class="floating-toggle"
      @click="toggle"
      :aria-expanded="isOpen.toString()"
      :aria-label="isOpen ? closeLabel : openLabel"
      :title="isOpen ? closeLabel : openLabel"
    >
      <FontAwesomeIcon
        v-if="!isOpen"
        :icon="faBars"
        class="floating-toggle__icon"
        aria-hidden="true"
      />
      <FontAwesomeIcon v-else :icon="faXmark" class="floating-toggle__icon" aria-hidden="true" />
    </button>
  </div>
</template>

<script>
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { faBookOpen } from '@fortawesome/free-solid-svg-icons/faBookOpen';
import { faChevronLeft } from '@fortawesome/free-solid-svg-icons/faChevronLeft';
import { faChevronRight } from '@fortawesome/free-solid-svg-icons/faChevronRight';
import { faScroll } from '@fortawesome/free-solid-svg-icons/faScroll';
import { faShieldHalved } from '@fortawesome/free-solid-svg-icons/faShieldHalved';
import { faTableCellsLarge } from '@fortawesome/free-solid-svg-icons/faTableCellsLarge';
import { faXmark } from '@fortawesome/free-solid-svg-icons/faXmark';

const faCharSheetTabIcons = {
  stats: faTableCellsLarge,
  bio: faBookOpen,
  gear: faShieldHalved,
  spells: faScroll,
};

function escapeRegExp(s) {
  return String(s).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

export default {
  name: "FloatingCard",
  props: {
    character: { type: Object, required: true },
    defaultOpen: { type: Boolean, default: false },
    hpSnapshot: { type: Object, default: null },
    /** When true, sheet is inline (e.g. setup confirm) — no FAB, no fixed position, not a modal dialog. */
    embedded: { type: Boolean, default: false },
  },
  computed: {
    language() {
      return (this.$store && this.$store.state && this.$store.state.language) || 'English';
    },
    statKeys() {
      return ['STR', 'DEX', 'CON', 'INT', 'WIS', 'CHA'];
    },
    displayRace() {
      return this.localizeSheetField(this.character.race, (this.$i18n.races || []));
    },
    displayClass() {
      const raw =
        this.character.characterClass != null && String(this.character.characterClass).trim()
          ? this.character.characterClass
          : this.character.class;
      return this.localizeSheetField(raw, (this.$i18n.classes || []));
    },
    displaySubrace() {
      const raw =
        this.character.subraceId != null && String(this.character.subraceId).trim()
          ? this.character.subraceId
          : this.character.subrace;
      if (raw == null || raw === '') return '';
      const labels = (this.$i18n && this.$i18n.subrace_labels) || {};
      const key = String(raw)
        .trim()
        .toLowerCase()
        .replace(/-/g, '_')
        .replace(/\s+/g, '_');
      if (labels[key]) return labels[key];
      return String(raw)
        .trim()
        .replace(/\*+$/u, '');
    },
    displaySubclass() {
      const raw =
        this.character.subclassId != null && String(this.character.subclassId).trim()
          ? this.character.subclassId
          : this.character.subclass;
      if (raw == null || raw === '') return '';
      const s = String(raw).trim();
      if (s.toLowerCase() === 'random') return '';
      const labels = (this.$i18n && this.$i18n.subclass_labels) || {};
      const keyUnderscore = s.toLowerCase().replace(/-/g, '_').replace(/\s+/g, '_');
      if (labels[keyUnderscore]) return labels[keyUnderscore];
      return s.replace(/\*+$/u, '').trim();
    },
    /** Normalized armor rows for display (strings or { name, ac_bonus, base_ac, ac } from server). */
    armorRows() {
      const a = this.character && this.character.armor;
      if (!Array.isArray(a) || !a.length) return [];
      return a.map((raw) => this.normalizeArmorDisplayRow(raw)).filter(Boolean);
    },
    /** General adventuring gear (not PHB Tools — see `toolsList`). */
    equipmentList() {
      const e = this.character && this.character.equipment;
      if (!Array.isArray(e) || !e.length) return [];
      return e.map((x) => String(x));
    },
    /** PHB-style tools / kits / instruments (separate from `equipment`). */
    toolsList() {
      const t = this.character && this.character.tools;
      if (!Array.isArray(t) || !t.length) return [];
      return t.map((x) => String(x).trim()).filter(Boolean);
    },
    languageList() {
      const l = this.character && this.character.languages;
      return Array.isArray(l) ? l.map((x) => String(x)) : [];
    },
    /**
     * Normalized spells; deduped by name+level; sorted by level then name.
     * Optional `prepared` (boolean) on leveled spells is preserved for spellbook-style sheets (D&D 5e).
     */
    spellsList() {
      const raw = this.character && this.character.spells;
      if (!Array.isArray(raw) || !raw.length) return [];
      const seen = new Set();
      const rows = [];
      for (const s of raw) {
        if (!s || typeof s !== 'object') continue;
        const name = String(s.name || '').trim();
        if (!name) continue;
        const lv = Math.max(0, Math.min(9, Math.floor(Number(s.level) || 0)));
        let key = name.toLowerCase();
        try {
          key = key.normalize('NFD').replace(/[\u0300-\u036f]/g, '');
        } catch (e) {
          /* ignore */
        }
        const dedupeKey = `${key}|${lv}`;
        if (seen.has(dedupeKey)) continue;
        seen.add(dedupeKey);
        const row = { name, level: lv };
        if (lv >= 1 && typeof s.prepared === 'boolean') {
          row.prepared = s.prepared;
        }
        rows.push(row);
      }
      rows.sort((a, b) => {
        if (a.level !== b.level) return a.level - b.level;
        return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
      });
      return rows;
    },
    /** True when any leveled spell carries an explicit `prepared` boolean (spellbook / dual-layer sheet). */
    spellsExplicitPreparedMode() {
      return this.spellsList.some(
        (s) => (Number(s.level) || 0) >= 1 && Object.prototype.hasOwnProperty.call(s, 'prepared')
      );
    },
    /** Parsed slot counts (includes zeros when the model sent that tier). */
    spellSlotsRaw() {
      const s = this.character && this.character.spell_slots;
      if (!s || typeof s !== 'object' || Array.isArray(s)) return null;
      const out = {};
      for (let L = 1; L <= 9; L++) {
        const k = String(L);
        if (s[k] == null) continue;
        const n = Math.floor(Number(s[k]));
        if (Number.isFinite(n) && n >= 0) out[k] = n;
      }
      return Object.keys(out).length ? out : null;
    },
    spellSlotsSummary() {
      const m = this.spellSlotsRaw;
      if (!m) return '';
      const parts = [];
      for (let L = 1; L <= 9; L++) {
        const k = String(L);
        if (m[k] == null || m[k] <= 0) continue;
        const ord = this.spellSlotOrdinalLabel(L);
        parts.push(`${ord}×${m[k]}`);
      }
      if (!parts.length) return '';
      const prefix = this.$i18n.spell_slots_prefix || '';
      return prefix ? `${prefix} ${parts.join(' · ')}` : parts.join(' · ');
    },
    hasLeveledSpells() {
      return this.spellsList.some((s) => (Number(s.level) || 0) >= 1);
    },
    /** Distinct spell levels on the sheet, ascending (0 = cantrips first). */
    spellLevelsSorted() {
      const set = new Set();
      for (const s of this.spellsList) {
        set.add(Math.max(0, Math.min(9, Number(s.level) || 0)));
      }
      return Array.from(set).sort((a, b) => a - b);
    },
    spellLevelsSortedSig() {
      return this.spellLevelsSorted.join(',');
    },
    spellLevelPageCurrent() {
      const lvls = this.spellLevelsSorted;
      if (!lvls.length) return null;
      const i = Math.min(Math.max(0, this.spellLevelPageIndex), lvls.length - 1);
      return lvls[i];
    },
    spellsOnCurrentLevelPage() {
      const L = this.spellLevelPageCurrent;
      if (L === null) return [];
      return this.spellsList.filter((s) => (Number(s.level) || 0) === L);
    },
    spellListAriaLabel() {
      const base = (this.$i18n && this.$i18n.sheet_spells) || 'Spells';
      const L = this.spellLevelPageCurrent;
      if (L === null) return base;
      return `${base}: ${this.spellLevelLabel(L)}`;
    },
    showWeaponsMissingHint() {
      const w = this.character && this.character.weapons;
      const hasRows = Array.isArray(w) && w.length > 0;
      const hasOther =
        this.armorRows.length > 0 || this.equipmentList.length > 0 || this.toolsList.length > 0;
      return !hasRows && hasOther;
    },
    hpDisplayText() {
      const c = this.character;
      const snap = this.hpSnapshot;
      let max = c && c.max_hp != null ? Number(c.max_hp) : null;
      let current = max;
      if (snap && typeof snap === 'object') {
        if (snap.max != null && !Number.isNaN(Number(snap.max))) max = Number(snap.max);
        if (snap.current != null && !Number.isNaN(Number(snap.current))) current = Number(snap.current);
        else if (max != null) current = max;
      }
      if (max == null || Number.isNaN(max)) return '';
      if (current == null || Number.isNaN(current)) current = max;
      const label = (this.$i18n && this.$i18n.hit_points_abbr) || 'HP';
      return `${current}/${max} ${label}`;
    },
    acDisplayText() {
      const ac = this.character && this.character.ac;
      if (ac == null || ac === '') return '';
      const n = Number(ac);
      if (Number.isNaN(n)) return '';
      const lab = (this.$i18n && this.$i18n.armor_class_abbr) || 'AC';
      return `${lab} ${n}`;
    },
    hpChipValue() {
      const t = this.hpDisplayText;
      if (!t) return '';
      const label = (this.$i18n && this.$i18n.hit_points_abbr) || 'HP';
      return t.replace(new RegExp(`\\s*${escapeRegExp(label)}\\s*$`, 'i'), '').trim();
    },
    acChipValue() {
      const t = this.acDisplayText;
      if (!t) return '';
      const lab = (this.$i18n && this.$i18n.armor_class_abbr) || 'AC';
      return t.replace(new RegExp(`^${escapeRegExp(lab)}\\s+`, 'i'), '').trim();
    },
    normalizedCoinage() {
      const keys = ['pp', 'gp', 'ep', 'sp', 'cp'];
      const out = { pp: 0, gp: 0, ep: 0, sp: 0, cp: 0 };
      const c = this.character && this.character.coinage;
      if (c && typeof c === 'object' && !Array.isArray(c)) {
        for (const k of keys) {
          const n = Math.floor(Number(c[k]));
          out[k] = Number.isFinite(n) && n >= 0 ? n : 0;
        }
      }
      return out;
    },
    /** PHB order: pp, gp, ep, sp, cp — one chip per denomination for the wallet strip. */
    coinageStrip() {
      const order = ['pp', 'gp', 'ep', 'sp', 'cp'];
      const cur = this.normalizedCoinage;
      const abbr = (this.$i18n && this.$i18n.coin_abbr) || {};
      return order.map((key) => ({
        key,
        amount: cur[key],
        abbr: abbr[key] || key,
      }));
    },
    charSheetTabs() {
      return [
        { id: 'stats', labelKey: 'sheet_tab_stats' },
        { id: 'bio', labelKey: 'sheet_tab_bio' },
        { id: 'gear', labelKey: 'sheet_tab_gear' },
        { id: 'spells', labelKey: 'sheet_spells' },
      ];
    },
  },
  data() {
    return {
      isOpen: this.embedded ? true : this.defaultOpen,
      titleId: 'floating-card-title-' + Math.random().toString(36).slice(2, 8),
      charSheetActiveTab: 0,
      spellLevelPageIndex: 0,
      openLabel: 'Open character sheet',
      closeLabel: 'Close character sheet',
      faCharSheetTabIcons,
      faChevronLeft,
      faChevronRight,
      faBars,
      faXmark,
    };
  },
  watch: {
    embedded(v) {
      if (v) this.isOpen = true;
    },
    character: {
      handler(newVal, oldVal) {
        if (!oldVal || !newVal) return;
        const sig = (c) =>
          [c.name, c.class, c.characterClass, c.level, c.subclass, c.subclassId, c.subrace, c.subraceId]
            .map((x) => String(x == null ? '' : x))
            .join('\0');
        if (sig(newVal) !== sig(oldVal)) {
          this.charSheetActiveTab = 0;
          this.spellLevelPageIndex = 0;
        }
      },
    },
    spellLevelsSortedSig(newVal, oldVal) {
      if (oldVal !== undefined && newVal !== oldVal) this.spellLevelPageIndex = 0;
      this.clampSpellLevelPageIndex();
    },
  },
  methods: {
    charSheetTabDomId(i) {
      return `${this.titleId}-tab-${i}`;
    },
    charSheetPanelDomId(i) {
      return `${this.titleId}-panel-${i}`;
    },
    tabLabel(tab) {
      const k = tab && tab.labelKey;
      const bundle = this.$i18n || {};
      return (k && bundle[k]) || k || '';
    },
    selectCharSheetTab(i) {
      const n = this.charSheetTabs.length;
      if (!Number.isFinite(i) || i < 0 || i >= n) return;
      this.charSheetActiveTab = i;
      this.$nextTick(() => {
        const el = this.$el && this.$el.querySelector && this.$el.querySelector(`#${this.charSheetPanelDomId(i)}`);
        if (el && typeof el.focus === 'function') el.focus();
      });
    },
    abilityModText(score) {
      const n = Number(score);
      if (!Number.isFinite(n)) return '';
      const m = Math.floor((n - 10) / 2);
      if (m >= 0) return `+${m}`;
      return String(m);
    },
    localizeSheetField(value, list) {
      if (value == null || value === '') return '';
      const raw = String(value).trim();
      const id = raw.toLowerCase().replace(/_/g, '-').replace(/\s+/g, '-');
      const found = list.find((x) => x.id === id);
      return found ? found.label : raw;
    },
    statAbbr(key) {
      const m = this.$i18n.statAbbr;
      return (m && m[key]) || key;
    },
    formatEquipmentQuantity(line) {
      const s = String(line == null ? '' : line).trim();
      const m = s.match(/^(.+?)\s*\((\d+)\)\s*$/);
      if (m && m[2] && m[2] !== '1') return `${m[2]}× ${m[1].trim()}`;
      return s;
    },
    /**
     * Armor row → { displayName, statsText } for sheet UI (matches server asArmorArray / parseArmorStringLine).
     */
    normalizeArmorDisplayRow(raw) {
      const acLab = (this.$i18n && this.$i18n.armor_class_abbr) || 'AC';
      if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
        const name = String(raw.name || '').trim();
        if (!name) return null;
        const displayName = this.formatEquipmentQuantity(name);
        let statsText = '';
        if (raw.ac != null && raw.ac !== '') {
          const n = Number(raw.ac);
          if (Number.isFinite(n)) statsText = `${acLab} ${n}`;
        } else if (raw.base_ac != null && raw.base_ac !== '') {
          const n = Number(raw.base_ac);
          if (Number.isFinite(n)) statsText = `${acLab} ${Math.floor(n)}`;
        } else if (raw.ac_bonus != null && raw.ac_bonus !== '') {
          const n = Number(String(raw.ac_bonus).replace(/^\+/, ''));
          if (Number.isFinite(n)) statsText = n >= 0 ? `+${n}` : String(n);
        } else if (/^(escudo|shield)\b/i.test(name)) {
          statsText = '+2';
        }
        return { displayName, statsText };
      }
      const s = String(raw == null ? '' : raw).trim();
      if (!s) return null;
      let name = s;
      let baseAc;
      let bonus;
      let m = s.match(/^(.*?)\s*\(\s*(?:AC|CA)\s*(\d+)\s*\)\s*$/i);
      if (m) {
        name = m[1].trim();
        baseAc = Number(m[2]);
      }
      if (baseAc == null) {
        m = s.match(/^(.*?)\s*\(\s*\+?\s*(\d+)\s*\)\s*$/);
        if (m) {
          const n = Number(m[2]);
          if (Number.isFinite(n)) {
            name = m[1].trim();
            bonus = n;
          }
        }
      }
      if (baseAc == null && bonus == null) {
        m = s.match(/^(.*?)[,;]?\s*\+(\d+)\s*(?:to\s*)?(?:AC|CA)?\s*$/i);
        if (m) {
          name = m[1].trim();
          bonus = Number(m[2]);
        }
      }
      if (!name) name = s;
      const displayName = this.formatEquipmentQuantity(name);
      let statsText = '';
      if (Number.isFinite(baseAc)) statsText = `${acLab} ${Math.floor(baseAc)}`;
      else if (Number.isFinite(bonus)) statsText = bonus >= 0 ? `+${bonus}` : String(bonus);
      else if (/^(escudo|shield)\b/i.test(name)) statsText = '+2';
      return { displayName, statsText };
    },
    weaponQty(w) {
      const q = w && w.quantity != null ? parseInt(String(w.quantity), 10) : NaN;
      if (Number.isFinite(q) && q >= 2) return q;
      return 1;
    },
    weaponBaseName(w) {
      const raw = String((w && w.name) || '').trim();
      return raw.replace(/^\d+\s*[x×]\s*/i, '').trim() || raw;
    },
    weaponDisplayName(w) {
      const base = this.weaponBaseName(w);
      const q = this.weaponQty(w);
      if (q >= 2 && !/^\d+\s*[x×]\s*/i.test(String(w.name || '').trim())) return `${q}× ${base}`;
      return String(w.name || '').trim() || base;
    },
    weaponDamageText(w) {
      const d = w && w.damage != null && String(w.damage).trim();
      return d || '—';
    },
    spellLevelLabel(level) {
      const L = Number(level) || 0;
      if (L === 0) return (this.$i18n && this.$i18n.spell_level_cantrip) || 'Cantrip';
      const pat = (this.$i18n && this.$i18n.spell_level_n) || 'Level {n}';
      return pat.replace(/\{n\}/g, String(L));
    },
    spellSlotOrdinalLabel(level) {
      const L = Number(level) || 1;
      const pat = (this.$i18n && this.$i18n.spell_slot_ordinal) || '{n}';
      return pat.replace(/\{n\}/g, String(L));
    },
    /**
     * D&D 5e: cantrips (level 0) are at-will. Leveled spells use shared spell slots; prepared casters with
     * a spellbook may list learned-but-unprepared spells — those stay visible but unhighlighted when `prepared` is explicit.
     */
    spellIsAtWill(spell) {
      return (Number(spell.level) || 0) === 0;
    },
    spellRowClassObj(spell) {
      const L = Number(spell.level) || 0;
      if (L === 0) return { 'cs-spell-row--at-will': true };
      if (!this.spellsExplicitPreparedMode) return { 'cs-spell-row--prepared': true };
      if (spell.prepared === true) return { 'cs-spell-row--prepared': true };
      return { 'cs-spell-row--learned-only': true };
    },
    clampSpellLevelPageIndex() {
      const n = this.spellLevelsSorted.length;
      if (!n) {
        this.spellLevelPageIndex = 0;
        return;
      }
      if (this.spellLevelPageIndex < 0) this.spellLevelPageIndex = 0;
      else if (this.spellLevelPageIndex >= n) this.spellLevelPageIndex = n - 1;
    },
    spellLevelPagePrev() {
      if (this.spellLevelPageIndex > 0) {
        this.spellLevelPageIndex--;
        this.clampSpellLevelPageIndex();
      }
    },
    spellLevelPageNext() {
      const n = this.spellLevelsSorted.length;
      if (this.spellLevelPageIndex < n - 1) {
        this.spellLevelPageIndex++;
        this.clampSpellLevelPageIndex();
      }
    },
    toggle() {
      if (this.embedded) return;
      this.isOpen = !this.isOpen;
      this.$nextTick(() => {
        if (!this.isOpen) {
          const btn = this.$el.querySelector('.floating-toggle');
          if (btn) btn.focus();
        } else {
          const panel = this.$el.querySelector('.floating-panel');
          if (panel) panel.focus();
        }
      });
    },
  },
};
</script>

<style src="./theme.css"></style>
