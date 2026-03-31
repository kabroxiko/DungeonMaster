<template>
    <div class="ui-panel game-load-panel">
        <h2 class="game-load-heading">{{ $i18n.load_game }}</h2>
        <p v-if="loading" class="game-load-status">{{ $i18n.loading }}</p>
        <p v-else-if="error" class="game-load-status game-load-error">{{ error }}</p>
        <p v-else-if="games.length === 0" class="game-load-status game-load-empty">{{ $i18n.load_game_empty }}</p>
        <ul v-else class="game-load-list">
            <li v-for="game in games" :key="game.gameId">
                <router-link class="game-load-link" :to="`/chat-room/${game.gameId}`">
                    <span class="game-load-title">{{ displayCampaignName(game) }}</span>
                </router-link>
            </li>
        </ul>
    </div>
</template>

<script>
import axios from 'axios';

export default {
    data() {
        return {
            games: [],
            loading: false,
            error: null,
        };
    },
    methods: {
        displayCampaignName(game) {
            if (!game) return '';
            const spec = game.campaignSpec;
            const t = spec && typeof spec.title === 'string' ? spec.title.trim() : '';
            if (t) return t;
            return game.gameId || '';
        },
    },
    async created() {
        this.loading = true;
        try {
            const response = await axios.get('/api/game-state/all');
            const data = response.data;
            this.games = Array.isArray(data) ? data : [];
        } catch (error) {
            this.error = this.$i18n.load_game_error;
        } finally {
            this.loading = false;
        }
    },
};
</script>

<style scoped>
/* Panel uses theme .ui-panel (dark surface + --gm-text). Links use high contrast vs parchment page behind. */
.game-load-panel {
    padding: 1.35rem 1.5rem 1.5rem;
    max-width: 560px;
    margin: 0 auto;
}

.game-load-heading {
    margin: 0 0 1rem 0;
    font-family: var(--gm-font-serif, Georgia, serif);
    font-size: 1.35rem;
    font-weight: 700;
    color: var(--gm-text, #e6e1d8);
    letter-spacing: 0.02em;
}

.game-load-status {
    margin: 0;
    color: var(--gm-text, #e6e1d8);
    font-size: 1rem;
    line-height: 1.5;
}

.game-load-error {
    color: #ffb4a8;
}

.game-load-empty {
    color: var(--gm-muted, #9a8f85);
    font-style: italic;
}

.game-load-list {
    list-style: none;
    padding: 0;
    margin: 0;
}

.game-load-list li {
    margin-bottom: 12px;
    line-height: 1.45;
}

.game-load-link {
    display: inline-block;
    color: #f2e6c9;
    text-decoration: underline;
    text-decoration-color: rgba(242, 230, 201, 0.55);
    text-underline-offset: 4px;
    font-weight: 600;
    font-size: 1.08rem;
    text-shadow: 0 1px 2px rgba(0, 0, 0, 0.85);
}

.game-load-link:hover {
    color: #fffdf6;
    text-decoration-color: rgba(255, 253, 246, 0.95);
}

.game-load-link:focus-visible {
    outline: 2px solid rgba(255, 220, 160, 0.95);
    outline-offset: 3px;
    border-radius: 2px;
}
</style>
