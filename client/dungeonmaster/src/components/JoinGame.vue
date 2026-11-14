<template>
  <div class="ui-panel join-game-panel">
    <h2 class="join-heading">{{ $i18n.join_game_title }}</h2>
    <p v-if="status" class="join-status">{{ status }}</p>
    <p v-if="error" class="join-status join-error">{{ error }}</p>
  </div>
</template>

<script>
import axios from 'axios';
import { SESSION_CHAT_TOAST, SESSION_SETUP_GAME_ID } from '@/setupSession.js';

export default {
  name: 'JoinGame',
  data() {
    return {
      status: '',
      error: null,
      redeeming: false,
      token: '',
    };
  },
  computed: {
    isAuthenticated() {
      return this.$store.getters.isAuthenticated;
    },
  },
  watch: {
    /**
     * Signing in from this page finishes the join by itself. The token is already in the URL (and in
     * session storage), so asking the player to reopen the link was pure busywork.
     */
    isAuthenticated(authed) {
      if (authed) void this.redeemInvite();
    },
  },
  mounted() {
    this.token = String(this.$route.params.token || '').trim();
    if (!this.token) {
      this.error = this.$i18n.join_game_missing_token;
      return;
    }
    try {
      sessionStorage.removeItem(SESSION_SETUP_GAME_ID);
      sessionStorage.setItem('dm_pending_invite', this.token);
    } catch (e) {
      /* ignore */
    }
    if (!this.isAuthenticated) {
      this.error = this.$i18n.join_game_sign_in_first;
      return;
    }
    void this.redeemInvite();
  },
  methods: {
    async redeemInvite() {
      const token = this.token || String(this.$route.params.token || '').trim();
      if (!token || this.redeeming) return;
      this.redeeming = true;
      this.status = this.$i18n.invite_joining_party;
      this.error = null;
      try {
        const { data } = await axios.post('/api/auth/join', { inviteToken: token });
        await this.enterParty(data);
      } catch (e) {
        const code = e?.response?.data?.code;
        if (code === 'AUTH_REQUIRED' || code === 'AUTH_INVALID') {
          // The stored session died. Drop it so the header offers sign-in again and keep the token pending,
          // because the watcher above redeems it the moment a valid session exists.
          void this.$store.dispatch('logout');
          this.error = this.$i18n.join_game_sign_in_first;
        } else {
          this.error = this.joinErrorMessage(e);
          this.clearPendingInvite();
        }
      } finally {
        this.redeeming = false;
        this.status = '';
      }
    },
    async enterParty(data) {
      const gameId = data?.gameId ? String(data.gameId) : '';
      if (!gameId) {
        this.error = this.$i18n.join_game_failed;
        return;
      }
      this.clearPendingInvite();
      this.$store.commit('setGameId', gameId);
      if (data.alreadyMember === true) this.toastAlreadyInParty();
      await this.$router.replace({ name: 'ChatRoomWithId', params: { id: gameId } });
    },
    joinErrorMessage(e) {
      const code = e?.response?.data?.code;
      if (code === 'INVITE_INVALID') return this.$i18n.join_invite_invalid;
      return e?.response?.data?.error || this.$i18n.join_game_failed;
    },
    toastAlreadyInParty() {
      try {
        sessionStorage.setItem(
          SESSION_CHAT_TOAST,
          JSON.stringify({ message: this.$i18n.join_already_in_party, variant: 'success' })
        );
      } catch (e) {
        /* ignore */
      }
    },
    clearPendingInvite() {
      try {
        sessionStorage.removeItem('dm_pending_invite');
      } catch (e) {
        /* ignore */
      }
    },
  },
};
</script>

<style scoped>
.join-game-panel {
  max-width: 480px;
  margin: 1rem auto;
  padding: 1.25rem 1.5rem;
}
.join-heading {
  margin: 0 0 0.75rem 0;
  font-family: var(--gm-font-serif, Georgia, serif);
  font-size: 1.25rem;
  color: var(--gm-text, #e6e1d8);
}
.join-status {
  margin: 0.5rem 0;
  color: var(--gm-text, #e6e1d8);
}
.join-error {
  color: #ffb4a8;
}
</style>
