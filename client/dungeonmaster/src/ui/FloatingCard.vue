<template>
  <div :class="['floating-card', { open: isOpen }]">
    <div
      class="floating-panel parchment-panel ornate-border"
      v-show="isOpen"
      role="dialog"
      aria-modal="true"
      :aria-hidden="(!isOpen).toString()"
      tabindex="-1"
      :aria-labelledby="titleId"
    >
      <div class="floating-header">
        <h2 :id="titleId" class="ui-heading">{{ character.name }}</h2>
        <div class="floating-sub">{{ displayRace }} • {{ displayClass }}{{ character.level ? (' — ' + $t('level_prefix') + character.level) : '' }}</div>
        <div v-if="combatStatsLine" class="floating-hp" aria-live="polite">{{ combatStatsLine }}</div>
      </div>
      <div class="floating-body">
        <div class="char-sheet">
          <div class="attributes-row" v-if="character.stats">
            <h3 class="cs-section-title">{{ $i18n.attributes }}</h3>
            <div class="attributes-grid">
              <div v-for="key in statKeys" :key="key" class="attr-box">
                <div class="attr-label">{{ statAbbr(key) }}</div>
                <div class="attr-value">{{ character.stats[key] }}</div>
              </div>
            </div>
          </div>

          <div class="char-history">
            <h3 class="cs-section-title">{{ $i18n.background }}</h3>
            <div class="cs-backstory" v-if="character.brief_backstory">{{ character.brief_backstory }}</div>
          </div>

          <div class="char-armor" v-if="armorList.length">
            <h3 class="cs-section-title">{{ $i18n.sheet_armor }}</h3>
            <ul>
              <li v-for="(it, i) in armorList" :key="'a'+i">{{ formatEquipmentQuantity(it) }}</li>
            </ul>
          </div>

          <div class="char-weapons" v-if="character.weapons && character.weapons.length">
            <h3 class="cs-section-title">{{ $i18n.weapons_sheet }}</h3>
            <ul>
              <li v-for="(w, i) in character.weapons" :key="'w'+i">
                <span class="weapon-name">{{ weaponDisplayName(w) }}</span>
                <span class="weapon-stats">
                  <template v-if="w.attack_bonus != null"> — +{{ w.attack_bonus }}</template>
                  <span> · {{ weaponDamageText(w) }}</span>
                  <template v-if="w.ability"> ({{ w.ability }})</template>
                </span>
              </li>
            </ul>
          </div>
          <div v-else-if="showWeaponsMissingHint" class="char-weapons char-weapons-missing">
            <h3 class="cs-section-title">{{ $i18n.weapons_sheet }}</h3>
            <p class="weapons-missing-hint">{{ $i18n.weapons_sheet_missing }}</p>
          </div>

          <div class="char-equipment" v-if="equipmentList.length">
            <h3 class="cs-section-title">{{ $i18n.equipment }}</h3>
            <ul>
              <li v-for="(it, i) in equipmentList" :key="'e'+i">{{ formatEquipmentQuantity(it) }}</li>
            </ul>
          </div>

          <div v-if="legacyStartingEquipment.length" class="char-equipment char-equipment-legacy">
            <h3 class="cs-section-title">{{ $i18n.equipment_legacy }}</h3>
            <ul>
              <li v-for="(it, i) in legacyStartingEquipment" :key="'l'+i">{{ formatEquipmentQuantity(it) }}</li>
            </ul>
          </div>

          <div class="char-languages" v-if="languageList.length">
            <h3 class="cs-section-title">{{ $i18n.sheet_languages }}</h3>
            <ul>
              <li v-for="(it, i) in languageList" :key="'lang'+i">{{ it }}</li>
            </ul>
          </div>
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
    /** Explicit equipment; falls back to legacy starting_equipment only when equipment absent. */
    /** Equipment lines plus legacy `tools` (same section — tools are not a separate sheet category). */
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
    /** Old saves: flat list when no explicit equipment array. */
    legacyStartingEquipment() {
      const hasExplicit = Array.isArray(this.character?.equipment) && this.character.equipment.length;
      if (hasExplicit) return [];
      const s = this.character && this.character.starting_equipment;
      return Array.isArray(s) ? s.map((x) => String(x)) : [];
    },
    languageList() {
      const l = this.character && this.character.languages;
      return Array.isArray(l) ? l.map((x) => String(x)) : [];
    },
    showWeaponsMissingHint() {
      const w = this.character && this.character.weapons;
      const hasRows = Array.isArray(w) && w.length > 0;
      const hasOther =
        this.armorList.length > 0 ||
        this.equipmentList.length > 0 ||
        this.legacyStartingEquipment.length > 0;
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
    combatStatsLine() {
      const hp = this.hpDisplayText;
      const ac = this.acDisplayText;
      if (hp && ac) return `${hp} · ${ac}`;
      return hp || ac || '';
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
