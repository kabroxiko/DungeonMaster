<template>
  <Teleport to="body">
    <UIModal v-if="modelValue" @close="close">
      <div
        class="settings-modal"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
      >
        <h2 :id="titleId" class="settings-modal__title">{{ $i18n.settings_title }}</h2>

        <div class="settings-modal__body">
          <section class="settings-modal__card" aria-labelledby="settings-lang-label">
            <h3 id="settings-lang-label" class="settings-modal__label">{{ $i18n.settings_language_label }}</h3>
            <div class="settings-modal__segment" role="group" :aria-label="$i18n.settings_language_label">
              <button
                type="button"
                class="settings-modal__opt"
                :class="{ active: draftLanguage === 'English' }"
                :aria-pressed="draftLanguage === 'English'"
                @click="draftLanguage = 'English'"
              >EN</button>
              <button
                type="button"
                class="settings-modal__opt"
                :class="{ active: draftLanguage === 'Spanish' }"
                :aria-pressed="draftLanguage === 'Spanish'"
                @click="draftLanguage = 'Spanish'"
              >ES</button>
            </div>
          </section>

          <section
            v-if="authenticated"
            class="settings-modal__card"
            aria-labelledby="settings-nick-label"
          >
            <h3 id="settings-nick-label" class="settings-modal__label">{{ $i18n.settings_nickname_label }}</h3>
            <p class="settings-modal__hint">{{ $i18n.settings_nickname_hint }}</p>
            <input
              v-model.trim="localNickname"
              type="text"
              class="settings-modal__input"
              maxlength="40"
              autocomplete="nickname"
              :aria-invalid="nickError ? 'true' : 'false'"
              @keyup.enter="saveAndClose"
            />
            <p v-if="nickError" class="settings-modal__error">{{ nickError }}</p>
          </section>
        </div>

        <footer class="settings-modal__footer">
          <p v-if="settingsSaved" class="settings-modal__ok" role="status">{{ $i18n.settings_saved }}</p>
          <div class="settings-modal__footer-actions">
            <button
              type="button"
              class="settings-modal__action settings-modal__action--secondary"
              :disabled="saveBusy"
              @click="applySettings"
            >
              {{ saveBusy ? $i18n.sending : $i18n.settings_apply }}
            </button>
            <UIButton
              class="settings-modal__action settings-modal__action--primary"
              :disabled="saveBusy"
              @click="saveAndClose"
            >
              {{ saveBusy ? $i18n.sending : $i18n.settings_save }}
            </UIButton>
          </div>
          <button
            v-if="authenticated"
            type="button"
            class="settings-modal__signout"
            :disabled="saveBusy"
            @click="requestSignOut"
          >
            <FontAwesomeIcon :icon="faRightFromBracket" class="settings-modal__signout-icon" aria-hidden="true" />
            {{ $i18n.sign_out }}
          </button>
        </footer>
      </div>
    </UIModal>
  </Teleport>
</template>

<script>
import UIModal from '@/ui/Modal.vue';
import UIButton from '@/ui/Button.vue';
import { faRightFromBracket } from '@fortawesome/free-solid-svg-icons/faRightFromBracket';

export default {
  name: 'SettingsModal',
  components: { UIModal, UIButton },
  props: {
    modelValue: { type: Boolean, default: false },
    authenticated: { type: Boolean, default: false },
  },
  emits: ['update:modelValue', 'language', 'nickname-saved', 'sign-out'],
  data() {
    return {
      faRightFromBracket,
      titleId: 'dm-settings-title',
      draftLanguage: 'English',
      localNickname: '',
      nickError: '',
      settingsSaved: false,
      saveBusy: false,
    };
  },
  watch: {
    modelValue: {
      immediate: true,
      handler(val) {
        if (val) {
          this.draftLanguage = this.$store.state.language === 'Spanish' ? 'Spanish' : 'English';
          this.syncNickname();
          this.nickError = '';
          this.settingsSaved = false;
          this.bindEsc();
        } else {
          this.unbindEsc();
        }
      },
    },
    localNickname() {
      this.settingsSaved = false;
    },
    draftLanguage() {
      this.settingsSaved = false;
    },
  },
  beforeUnmount() {
    this.unbindEsc();
  },
  methods: {
    bindEsc() {
      document.addEventListener('keydown', this.onDocKeydown);
    },
    unbindEsc() {
      document.removeEventListener('keydown', this.onDocKeydown);
    },
    onDocKeydown(e) {
      if (e.key === 'Escape') this.close();
    },
    close() {
      this.$emit('update:modelValue', false);
    },
    /** Signing out is the caller's job (it owns the route change and the store reset). */
    requestSignOut() {
      if (this.saveBusy) return;
      this.$emit('sign-out');
    },
    syncNickname() {
      const u = this.$store.state.user;
      this.localNickname = u && u.nickname ? String(u.nickname).trim() : '';
    },
    applySettings() {
      return this.persistSettings(false);
    },
    saveAndClose() {
      return this.persistSettings(true);
    },
    async persistSettings(closeAfter) {
      if (this.saveBusy) return;
      this.nickError = '';
      if (!closeAfter) {
        this.settingsSaved = false;
      }

      if (this.authenticated) {
        const n = String(this.localNickname || '').trim();
        if (!n.length) {
          this.nickError = this.$i18n.nickname_error_empty;
          return;
        }
        if (n.length > 40) {
          this.nickError = this.$i18n.nickname_error_length;
          return;
        }
      }

      const langChanged = this.draftLanguage !== this.$store.state.language;
      let nickChanged = false;
      if (this.authenticated) {
        const n = String(this.localNickname || '').trim();
        const current = (this.$store.state.user && this.$store.state.user.nickname) || '';
        nickChanged = n !== String(current).trim();
      }

      if (!langChanged && !nickChanged) {
        if (closeAfter) {
          this.close();
        }
        return;
      }

      this.saveBusy = true;
      try {
        if (langChanged) {
          this.$emit('language', this.draftLanguage);
        }
        if (this.authenticated && nickChanged) {
          await this.$store.dispatch('saveNickname', String(this.localNickname || '').trim());
          this.$emit('nickname-saved');
        }
        if (closeAfter) {
          this.close();
        } else {
          this.settingsSaved = true;
        }
      } catch (e) {
        this.nickError = e.response?.data?.error || e.message || this.$i18n.nickname_error_save;
      } finally {
        this.saveBusy = false;
      }
    },
  },
};
</script>

<style scoped>
.settings-modal {
  padding: 0.15rem 0.25rem 0.15rem 0;
  max-width: 22.5rem;
  margin: 0 auto;
}

.settings-modal__title {
  margin: 0 0 1rem 0;
  font-family: var(--gm-font-serif, Georgia, serif);
  font-size: 1.35rem;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--gm-gold, #d4b46a);
  padding-right: 2rem;
  line-height: 1.2;
}

.settings-modal__body {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.settings-modal__card {
  padding: 0.95rem 1rem;
  border-radius: 12px;
  background: linear-gradient(165deg, rgba(255, 255, 255, 0.03), rgba(0, 0, 0, 0.22));
  border: 1px solid rgba(212, 180, 106, 0.12);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.settings-modal__label {
  margin: 0 0 0.65rem 0;
  font-size: 0.72rem;
  font-weight: 700;
  color: var(--gm-gold, #d4b46a);
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.settings-modal__hint {
  margin: 0 0 0.55rem 0;
  font-size: 0.84rem;
  line-height: 1.5;
  color: var(--gm-muted, #9a8f85);
}

.settings-modal__segment {
  display: inline-flex;
  padding: 4px;
  border-radius: 11px;
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: inset 0 2px 6px rgba(0, 0, 0, 0.35);
}

.settings-modal__opt {
  min-width: 3.35rem;
  padding: 11px 16px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--gm-muted);
  font-size: 0.82rem;
  font-weight: 700;
  letter-spacing: 0.14em;
  cursor: pointer;
  transition: background 0.14s ease, color 0.14s ease, box-shadow 0.14s ease;
}

.settings-modal__opt:hover {
  color: var(--gm-text);
}

.settings-modal__opt:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px rgba(212, 180, 106, 0.45);
  border-radius: 8px;
}

.settings-modal__opt.active {
  background: linear-gradient(180deg, rgba(212, 180, 106, 0.28), rgba(212, 180, 106, 0.12));
  color: var(--gm-gold-bright, #f0dfa8);
  box-shadow:
    inset 0 0 0 1px rgba(212, 180, 106, 0.5),
    0 1px 8px rgba(0, 0, 0, 0.25);
}

.settings-modal__input {
  width: 100%;
  box-sizing: border-box;
  padding: 0.65rem 0.8rem;
  margin: 0;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.35);
  color: var(--gm-text);
  font-size: 1rem;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.settings-modal__input:hover {
  border-color: rgba(212, 180, 106, 0.2);
}

.settings-modal__input:focus {
  outline: none;
  border-color: rgba(212, 180, 106, 0.42);
  box-shadow: 0 0 0 3px rgba(212, 180, 106, 0.12);
}

.settings-modal__error {
  margin: 0.5rem 0 0 0;
  font-size: 0.84rem;
  color: #ffb4a8;
}

.settings-modal__footer {
  margin-top: 1.15rem;
  padding-top: 1.1rem;
  border-top: 1px solid rgba(212, 180, 106, 0.14);
}

.settings-modal__ok {
  margin: 0 0 0.75rem 0;
  font-size: 0.88rem;
  color: #a8e6a1;
  text-align: center;
}

.settings-modal__footer-actions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.65rem 0.85rem;
  align-items: stretch;
}

.settings-modal__action {
  width: 100%;
  min-height: 2.85rem;
  padding: 0.65rem 0.75rem;
  border-radius: var(--gm-radius, 10px);
  font-weight: 600;
  font-size: 0.9rem;
  letter-spacing: 0.02em;
  cursor: pointer;
  transition: transform 0.1s ease, box-shadow 0.1s ease, border-color 0.12s ease, background 0.12s ease, opacity 0.12s ease;
}

.settings-modal__action--secondary {
  font-family: inherit;
  background: rgba(0, 0, 0, 0.28);
  border: 1px solid rgba(212, 180, 106, 0.38);
  color: var(--gm-gold-bright, #f0dfa8);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05);
}

.settings-modal__action--secondary:hover:not(:disabled) {
  border-color: rgba(212, 180, 106, 0.58);
  background: rgba(212, 180, 106, 0.1);
  transform: translateY(-1px);
}

.settings-modal__action--secondary:active:not(:disabled) {
  transform: translateY(0);
}

.settings-modal__action--secondary:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px rgba(212, 180, 106, 0.35);
}

.settings-modal__action--secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  transform: none;
}

.settings-modal__action--primary {
  box-sizing: border-box;
  width: 100%;
  min-height: 2.85rem;
  padding-top: 0.65rem;
  padding-bottom: 0.65rem;
  font-size: 0.9rem;
}

/* Account action: separated from Apply/Save so it never reads as another way to keep the settings. */
.settings-modal__signout {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  width: 100%;
  margin-top: 0.9rem;
  min-height: 2.85rem;
  padding: 0.65rem 0.75rem;
  border-radius: var(--gm-radius, 10px);
  font-family: inherit;
  font-weight: 600;
  font-size: 0.9rem;
  letter-spacing: 0.02em;
  cursor: pointer;
  background: rgba(0, 0, 0, 0.28);
  border: 1px solid rgba(255, 140, 120, 0.38);
  color: #ffd4ce;
  transition: transform 0.1s ease, border-color 0.12s ease, background 0.12s ease, opacity 0.12s ease;
}

.settings-modal__signout:hover:not(:disabled) {
  border-color: rgba(255, 140, 120, 0.62);
  background: rgba(255, 140, 120, 0.1);
  transform: translateY(-1px);
}

.settings-modal__signout:active:not(:disabled) {
  transform: translateY(0);
}

.settings-modal__signout:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px rgba(255, 140, 120, 0.4);
}

.settings-modal__signout:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  transform: none;
}

.settings-modal__signout-icon {
  width: 0.95rem;
  height: 0.95rem;
}

@media (max-width: 360px) {
  .settings-modal__footer-actions {
    grid-template-columns: 1fr;
  }
}
</style>
