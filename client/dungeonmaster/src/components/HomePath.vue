<template>
  <div class="public-landing ui-panel">
    <p class="public-lead">{{ $i18n.public_hero_title }}</p>
    <div class="public-body">
      <p>{{ $i18n.public_intro_p1 }}</p>
      <p>{{ $i18n.public_intro_p2 }}</p>
    </div>
    <h2 class="public-subheading">{{ $i18n.public_features_heading }}</h2>
    <ul class="public-list">
      <li v-for="(line, i) in featureLines" :key="i">{{ line }}</li>
    </ul>
    <p v-if="!googleClientId" class="home-hint">{{ $i18n.auth_env_hint }}</p>
    <p class="public-footer">{{ $i18n.public_footer_note }}</p>
  </div>
</template>

<script>
export default {
  name: 'HomePath',
  computed: {
    googleClientId() {
      return this.$store.state.googleClientId || '';
    },
    featureLines() {
      const f = this.$i18n.public_features;
      return Array.isArray(f) ? f : [];
    },
  },
};
</script>

<style scoped>
.public-landing {
  padding: 1.35rem 1.5rem 1.75rem;
  max-width: 640px;
  margin: 0 auto;
}
.public-lead {
  font-family: var(--gm-font-serif, Georgia, serif);
  font-size: 1.2rem;
  font-weight: 600;
  color: var(--gm-text, #e6e1d8);
  margin: 0 0 1rem 0;
  line-height: 1.4;
}
.public-body p,
.public-footer {
  color: var(--gm-text, #e6e1d8);
  line-height: 1.55;
  margin: 0 0 0.85rem 0;
  font-size: 1rem;
}
.public-subheading {
  font-family: var(--gm-font-serif, Georgia, serif);
  font-size: 1.05rem;
  margin: 1.25rem 0 0.6rem 0;
  color: var(--gm-text, #e6e1d8);
}
.public-list {
  margin: 0 0 1rem 0;
  padding-left: 1.25rem;
  color: var(--gm-text, #e6e1d8);
  line-height: 1.5;
}
.public-list li {
  margin-bottom: 0.35rem;
}
.home-hint {
  margin-top: 1rem;
  line-height: 1.5;
  color: var(--gm-muted, #a8a29a);
  font-size: 0.95rem;
}
.public-footer {
  margin-top: 1.5rem;
  font-size: 0.9rem;
  color: var(--gm-muted, #a8a29a);
}
</style>
