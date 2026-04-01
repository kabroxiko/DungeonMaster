<template>
  <div :class="['floating-card', { open: isOpen }]">
    <div
      class="floating-panel parchment-panel ornate-border char-sheet-panel"
      v-show="isOpen"
      role="dialog"
      aria-modal="true"
      :aria-hidden="(!isOpen).toString()"
      tabindex="-1"
      :aria-labelledby="titleId"
    >
      <header class="floating-header char-sheet-header">
        <div class="char-sheet-hero">
          <h2 :id="titleId" class="ui-heading char-sheet-name">{{ character.name }}</h2>
          <p class="floating-sub char-sheet-lineage">
            <span class="char-sheet-lineage-main">{{ displayRace }} · {{ displayClass }}</span>
            <template v-if="character.subclass">
              <span class="char-sheet-subclass"> · {{ character.subclass }}</span>
            </template>
            <template v-if="character.level">
              <span class="char-sheet-level"> — {{ $t('level_prefix') }}{{ character.level }}</span>
            </template>
          </p>
          <p v-if="character.background" class="char-sheet-bg-role">{{ character.background }}</p>
        </div>
        <div
          class="cs-vitals"
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
      </header>
      <div class="floating-body char-sheet-body">
        <div class="char-sheet">
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

          <section v-if="character.brief_backstory" class="cs-block cs-block--story">
            <h3 class="cs-section-title">{{ $i18n.sheet_story }}</h3>
            <div class="cs-backstory">{{ character.brief_backstory }}</div>
          </section>

          <section v-if="armorList.length" class="cs-block cs-block--armor">
            <h3 class="cs-section-title">{{ $i18n.sheet_armor }}</h3>
            <ul class="cs-list cs-list--plain">
              <li v-for="(it, i) in armorList" :key="'a'+i">{{ formatEquipmentQuantity(it) }}</li>
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

          <section v-if="languageList.length" class="cs-block cs-block--langs">
            <h3 class="cs-section-title">{{ $i18n.sheet_languages }}</h3>
            <div class="cs-lang-tags">
              <span v-for="(it, i) in languageList" :key="'lang'+i" class="cs-lang-tag">{{ it }}</span>
            </div>
          </section>
        </div>
      </div>
    </div>

    <button class="floating-toggle" @click="toggle" :aria-expanded="isOpen.toString()" :title="isOpen ? closeLabel : openLabel">
      <span v-if="!isOpen">☰</span>
      <span v-else>✕</span>
    </button>
  </div>
</template>

<script>
function escapeRegExp(s) {
  return String(s).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

export default {
  name: "FloatingCard",
  props: {
    character: { type: Object, required: true },
    defaultOpen: { type: Boolean, default: false },
    hpSnapshot: { type: Object, default: null },
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
      return this.localizeSheetField(this.character.class, (this.$i18n.classes || []));
    },
    /** Explicit field from generator; [] if missing. */
    armorList() {
      const a = this.character && this.character.armor;
      return Array.isArray(a) ? a.map((x) => String(x)) : [];
    },
    /** Equipment lines plus `tools` when present (merged for display — tools are not a separate sheet category). */
    equipmentList() {
      const c = this.character;
      const e = c && c.equipment;
      const t = c && c.tools;
      const list = Array.isArray(e) && e.length ? e.map((x) => String(x)) : [];
      if (!Array.isArray(t) || !t.length) return list;
      const seen = new Set(list.map((s) => s.toLowerCase()));
      for (const x of t) {
        const s = String(x).trim();
        if (!s) continue;
        const k = s.toLowerCase();
        if (!seen.has(k)) {
          seen.add(k);
          list.push(s);
        }
      }
      return list;
    },
    languageList() {
      const l = this.character && this.character.languages;
      return Array.isArray(l) ? l.map((x) => String(x)) : [];
    },
    showWeaponsMissingHint() {
      const w = this.character && this.character.weapons;
      const hasRows = Array.isArray(w) && w.length > 0;
      const hasOther = this.armorList.length > 0 || this.equipmentList.length > 0;
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
  },
  data() {
    return {
      isOpen: this.defaultOpen,
      titleId: 'floating-card-title-' + Math.random().toString(36).slice(2, 8),
      openLabel: 'Open character sheet',
      closeLabel: 'Close character sheet',
    };
  },
  methods: {
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
    toggle() {
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
