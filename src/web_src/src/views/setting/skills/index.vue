<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useBoolean } from '@/hooks'
import { fetchAgentSkill, fetchAgentSkills } from '@/api/agent'

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(true)

const items = ref([])
const total = ref(0)
const domains = ref([])
const library = ref(null)

const search = ref('')
const domain = ref(null)
const scope = ref('all')
const page = ref(1)
const pageSize = 25

const scopeOptions = [
  { label: 'All', value: 'all' },
  { label: 'Built-in', value: 'global' },
  { label: 'Mine', value: 'user' },
]

// The library is far too large to filter client-side, so every control maps to
// a query parameter and the server does the work.
const domainOptions = computed(() =>
  domains.value.map(d => ({ label: `${d.domain} (${d.count})`, value: d.domain })),
)

async function load() {
  startLoading()
  try {
    const { isSuccess, data } = await fetchAgentSkills({
      search: search.value || undefined,
      domain: domain.value || undefined,
      scope: scope.value === 'all' ? undefined : scope.value,
      limit: pageSize,
      offset: (page.value - 1) * pageSize,
    })
    if (isSuccess) {
      items.value = data?.items || []
      total.value = data?.total || 0
      // Facets and provenance are stable; they ride along with every response
      // so the filter list never goes stale after a bundle update.
      domains.value = data?.domains || []
      library.value = data?.library || null
    }
  }
  finally {
    endLoading()
  }
}

// Any filter change resets to the first page — staying on page 7 of a result set
// that just shrank to 3 entries shows an empty list.
let searchTimer = null
watch(search, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    load()
  }, 300)
})
watch([domain, scope], () => {
  page.value = 1
  load()
})
watch(page, load)

const detail = ref(null)
const { bool: showDetail, setTrue: openDetail, setFalse: closeDetail } = useBoolean(false)
const { bool: detailLoading, setTrue: startDetail, setFalse: endDetail } = useBoolean(false)

async function openSkill(name) {
  openDetail()
  detail.value = null
  startDetail()
  try {
    const { isSuccess, data } = await fetchAgentSkill(name)
    if (isSuccess)
      detail.value = data
  }
  finally {
    endDetail()
  }
}

onMounted(load)
</script>

<template>
  <div>
    <n-space vertical size="large">
      <n-alert type="info" title="Agent Skills">
        Skills are domain-specific instructions and scripts the agent runs via
        <code>workspace_exec</code>. The built-in library ships inside Antelope, so it
        is versioned with the release — nothing to install or mount.
        <template v-if="library">
          <br>
          <n-text depth="3">
            {{ library.skills }} built-in skills from
            <template v-for="(src, i) in library.sources" :key="src.name">
              <a v-if="src.url" :href="src.url" target="_blank" rel="noopener">{{ src.name }}</a>
              <span v-else>{{ src.name }}</span>
              <n-text depth="3"> ({{ src.license }})</n-text>{{ i < library.sources.length - 1 ? ', ' : '' }}
            </template>
          </n-text>
        </template>
        <br>
        <n-text depth="3">
          The agent does not see this list in its prompt — it searches the library by
          keyword and loads what it needs, which is why the catalogue can be this large.
        </n-text>
      </n-alert>

      <n-card title="Skills">
        <template #header-extra>
          <n-space align="center">
            <n-input
              v-model:value="search"
              placeholder="Search skills…"
              clearable
              size="small"
              style="width: 220px"
            />
            <n-select
              v-model:value="domain"
              :options="domainOptions"
              placeholder="All domains"
              clearable
              filterable
              size="small"
              style="width: 200px"
            />
            <n-radio-group v-model:value="scope" size="small">
              <n-radio-button
                v-for="opt in scopeOptions"
                :key="opt.value"
                :value="opt.value"
              >
                {{ opt.label }}
              </n-radio-button>
            </n-radio-group>
          </n-space>
        </template>

        <n-spin :show="loading">
          <n-empty v-if="!items.length && !loading" description="No skills match this filter">
            <template #icon>
              <icon-park-outline-bookmark class="text-4xl text-gray-400" />
            </template>
          </n-empty>

          <n-list v-else bordered clickable hoverable>
            <n-list-item
              v-for="it in items"
              :key="it.name"
              style="cursor: pointer"
              @click="openSkill(it.name)"
            >
              <n-thing>
                <template #header>
                  <n-space align="center">
                    <n-text strong>
                      {{ it.name }}
                    </n-text>
                    <n-tag v-if="it.is_global" type="info" size="small">
                      Built-in
                    </n-tag>
                    <n-tag v-if="it.domain" size="small">
                      {{ it.domain }}
                    </n-tag>
                    <n-tag v-if="it.version" type="default" size="small">
                      v{{ it.version }}
                    </n-tag>
                  </n-space>
                </template>
                <template #description>
                  <n-ellipsis :line-clamp="2">
                    <n-text depth="3">
                      {{ it.description || 'No description.' }}
                    </n-text>
                  </n-ellipsis>
                </template>
              </n-thing>
            </n-list-item>
          </n-list>

          <n-space v-if="total > pageSize" justify="center" style="margin-top: 16px">
            <n-pagination
              v-model:page="page"
              :page-size="pageSize"
              :item-count="total"
              :page-slot="7"
            />
          </n-space>
          <n-text v-if="total" depth="3" style="display: block; margin-top: 12px">
            {{ total }} skill{{ total === 1 ? '' : 's' }} match
          </n-text>
        </n-spin>
      </n-card>
    </n-space>

    <n-drawer :show="showDetail" :width="640" @update:show="v => v || closeDetail()">
      <n-drawer-content :title="detail?.name || 'Skill'" closable :native-scrollbar="false">
        <n-spin :show="detailLoading">
          <n-space vertical size="large">
            <n-space align="center">
              <n-tag v-if="detail?.domain" size="small">
                {{ detail.domain }}
              </n-tag>
              <n-tag v-if="detail?.source" type="info" size="small">
                {{ detail.source }}
              </n-tag>
              <n-tag v-for="tag in detail?.tags || []" :key="tag" size="small" type="success">
                {{ tag }}
              </n-tag>
            </n-space>
            <n-text depth="3">
              {{ detail?.description }}
            </n-text>
            <!--
              The raw SKILL.md, i.e. exactly the instructions the agent loads.
              Deliberately unrendered: a plain <pre> is honest about being the
              source file, and n-code would need hljs wired into
              NConfigProvider, which this app does not do.
            -->
            <pre v-if="detail?.body" class="skill-body">{{ detail.body }}</pre>
            <n-text v-else-if="!detailLoading" depth="3">
              No SKILL.md body available for this skill.
            </n-text>
          </n-space>
        </n-spin>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.skill-body {
  margin: 0;
  padding: 12px;
  border-radius: 4px;
  background-color: var(--n-color-embedded, rgba(128, 128, 128, 0.1));
  font-family: var(--n-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
