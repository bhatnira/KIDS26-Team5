<script setup>
import { computed } from 'vue'

const props = defineProps({
  conversations: {
    type: Array,
    default: () => [],
  },
  activeId: {
    // Conversation IDs are framework session UUID strings, not numbers.
    type: String,
    default: null,
  },
  loading: {
    type: Boolean,
    default: false,
  },
  hasMore: {
    type: Boolean,
    default: false,
  },
  loadingMore: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['select', 'delete', 'new', 'load-more'])

// Infinite scroll: when the list is scrolled near the bottom and more pages
// remain, ask the parent to load the next batch.
function handleScroll(e) {
  if (!props.hasMore || props.loadingMore) return
  const el = e.target
  if (el.scrollHeight - el.scrollTop - el.clientHeight < 80) {
    emit('load-more')
  }
}

function formatDate(dateStr) {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now - date
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffDays === 0) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }
  if (diffDays === 1) return 'Yesterday'
  if (diffDays < 7) return `${diffDays}d ago`
  return date.toLocaleDateString()
}
</script>

<template>
  <div class="conversation-list">
    <!-- Header -->
    <div class="conversation-list__header">
      <span class="conversation-list__title">Conversations</span>
      <n-button
        quaternary
        circle
        size="small"
        title="New conversation"
        @click="emit('new')"
      >
        <template #icon>
          <icon-park-outline-add />
        </template>
      </n-button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="conversation-list__loading">
      <n-spin size="small" />
    </div>

    <!-- Empty state -->
    <div v-else-if="conversations.length === 0" class="conversation-list__empty">
      <n-empty size="small" description="No conversations yet">
        <template #extra>
          <n-button size="small" type="primary" @click="emit('new')">
            Start chatting
          </n-button>
        </template>
      </n-empty>
    </div>

    <!-- List -->
    <div v-else class="conversation-list__items" @scroll="handleScroll">
      <div
        v-for="conv in conversations"
        :key="conv.id"
        class="conv-item"
        :class="{ 'conv-item--active': conv.id === activeId }"
        @click="emit('select', conv.id)"
      >
        <div class="conv-item__icon">
          <icon-park-outline-message-one class="text-sm" />
        </div>
        <div class="conv-item__body">
          <div class="conv-item__title">{{ conv.title || 'New Conversation' }}</div>
          <div class="conv-item__time">{{ formatDate(conv.updated_at) }}</div>
        </div>
        <div class="conv-item__actions" @click.stop>
          <n-popconfirm
            placement="right"
            @positive-click="emit('delete', conv.id)"
          >
            <template #trigger>
              <n-button
                text
                size="tiny"
                class="conv-item__delete"
                title="Delete conversation"
              >
                <template #icon>
                  <icon-park-outline-delete />
                </template>
              </n-button>
            </template>
            Delete this conversation?
          </n-popconfirm>
        </div>
      </div>

      <!-- Infinite-scroll footer -->
      <div v-if="loadingMore" class="conversation-list__more">
        <n-spin size="small" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.conversation-list {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.conversation-list__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 12px 8px;
  flex-shrink: 0;
  border-bottom: 1px solid var(--n-border-color, rgba(0, 0, 0, 0.09));
}

.conversation-list__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--n-text-color-2);
  letter-spacing: 0.3px;
  text-transform: uppercase;
}

.conversation-list__loading {
  display: flex;
  justify-content: center;
  padding: 20px;
}

.conversation-list__empty {
  display: flex;
  justify-content: center;
  padding: 20px 12px;
}

.conversation-list__items {
  flex: 1;
  overflow-y: auto;
  padding: 6px 6px;
}

.conversation-list__more {
  display: flex;
  justify-content: center;
  padding: 10px 0;
}

.conv-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s;
  position: relative;
}

.conv-item:hover {
  background: var(--n-color-hover, rgba(0, 0, 0, 0.06));
}

.conv-item--active {
  background: var(--n-primary-color-suppl, rgba(32, 128, 240, 0.12));
}

.conv-item--active .conv-item__title {
  color: var(--n-primary-color, #2080f0);
}

.conv-item__icon {
  color: var(--n-text-color-3);
  flex-shrink: 0;
  font-size: 14px;
}

.conv-item__body {
  flex: 1;
  overflow: hidden;
}

.conv-item__title {
  font-size: 13px;
  font-weight: 500;
  color: var(--n-text-color);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.conv-item__time {
  font-size: 11px;
  color: var(--n-text-color-3);
  margin-top: 1px;
}

.conv-item__actions {
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.15s;
}

.conv-item:hover .conv-item__actions {
  opacity: 1;
}

.conv-item__delete {
  color: var(--n-text-color-3) !important;
}

.conv-item__delete:hover {
  color: var(--n-error-color, #d03050) !important;
}
</style>
