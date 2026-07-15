<template>
  <div class="compass">
    <aside class="sidebar">
      <div class="brand">
        <img class="logo" src="./assets/appicon.png" alt="Evelent DB" width="40" height="40" />
        <div>
          <div class="brand-title">Evelent DB</div>
          <div class="brand-sub">{{ displayURL }}</div>
        </div>
      </div>
      <div class="sidebar-section conn-box">
        <div class="sidebar-label">Сервер API</div>
        <input
          v-model="serverURLInput"
          class="conn-url"
          type="text"
          spellcheck="false"
          placeholder="http://127.0.0.1:8080"
          title="Полный URL или host:port"
        />
        <input
          v-model="apiKeyInput"
          class="conn-url"
          type="password"
          spellcheck="false"
          placeholder="API key (X-API-Key)"
          title="API key for authenticated servers"
        />
        <div class="conn-actions">
          <button class="btn-sm primary" type="button" @click="applyServerURL">Применить</button>
          <button class="btn-sm" type="button" @click="testConnection">Проверить</button>
        </div>
        <div class="conn-pill active">HTTP · REST</div>
      </div>
      <div class="mode-switch">
        <button :class="{ on: mode === 'collections' }" type="button" @click="mode = 'collections'">
          ▤ Коллекции
        </button>
        <button :class="{ on: mode === 'kv' }" type="button" @click="switchToKV">
          ⚡ KV-хранилище
        </button>
      </div>
      <div v-show="mode === 'collections'" class="sidebar-section grow">
        <div class="row-between">
          <span class="sidebar-label">Коллекции</span>
          <button class="icon-btn" type="button" title="Обновить" @click="refreshCollections">↻</button>
        </div>
        <div class="new-coll">
          <input v-model="newCollName" placeholder="Новая коллекция" @keyup.enter="createCollection" />
          <button class="btn-sm primary" type="button" @click="createCollection">+</button>
        </div>
        <ul class="coll-list">
          <li
            v-for="c in collections"
            :key="c"
            :class="{ active: c === selectedCollection }"
            @click="selectCollection(c)"
          >
            <span class="coll-icon">▤</span>
            <span class="coll-name">{{ c }}</span>
            <button
              class="coll-del"
              type="button"
              title="Удалить коллекцию"
              @click.stop="dropCollection(c)"
            >
              ×
            </button>
          </li>
        </ul>
      </div>
    </aside>

    <main class="main">
      <header class="topbar" v-if="mode === 'collections'">
        <div class="breadcrumb">
          <span class="crumb">Evelent</span>
          <span class="sep">/</span>
          <span class="crumb">{{ selectedCollection || "…" }}</span>
          <span class="sep">/</span>
          <span class="crumb dim">{{ activeTab }}</span>
        </div>
        <div class="tabs">
          <button :class="{ on: activeTab === 'Документы' }" type="button" @click="activeTab = 'Документы'">
            Документы
          </button>
          <button :class="{ on: activeTab === 'Индексы' }" type="button" @click="activeTab = 'Индексы'">
            Индексы
          </button>
        </div>
      </header>
      <header class="topbar" v-else>
        <div class="breadcrumb">
          <span class="crumb">Evelent</span>
          <span class="sep">/</span>
          <span class="crumb dim">In-Memory KV</span>
        </div>
        <div v-if="kvStats" class="kv-stats-inline">
          <span>{{ kvStats.items }} ключей</span>
          <span class="muted">·</span>
          <span>{{ formatBytes(Number(kvStats.bytes)) }}</span>
          <span class="muted">·</span>
          <span>hit {{ (Number(kvStats.hitRatio) * 100).toFixed(1) }}%</span>
        </div>
      </header>

      <div v-if="mode === 'collections' && !selectedCollection" class="empty-main">
        <h2>Выберите коллекцию</h2>
        <p class="muted">Слева список коллекций — как в MongoDB Compass.</p>
      </div>

      <div v-else-if="mode === 'collections'" class="content">
        <div v-if="activeTab === 'Документы'" class="panel">
          <div class="stats-row" v-if="stats">
            <span>{{ stats.count }} док.</span>
            <span class="muted">·</span>
            <span>{{ formatBytes(stats.storageSize) }}</span>
            <button class="btn-sm" type="button" @click="loadStats">Обновить статистику</button>
          </div>
          <div class="toolbar">
            <button class="btn primary" type="button" @click="runQuery">Найти</button>
            <button class="btn" type="button" @click="loadAll">Показать все</button>
            <button class="btn" type="button" @click="showInsert = true">Добавить данные</button>
            <button class="btn" type="button" @click="exportDocs">Экспорт JSON</button>
          </div>
          <div class="query-grid">
            <label>
              <span class="lbl">Фильтр (JSON)</span>
              <textarea v-model="filterJSON" rows="4" placeholder="{}" />
            </label>
            <div class="query-opts">
              <label>
                <span class="lbl">Limit</span>
                <input v-model.number="queryLimit" type="number" min="0" />
              </label>
              <label>
                <span class="lbl">Skip</span>
                <input v-model.number="querySkip" type="number" min="0" />
              </label>
              <label>
                <span class="lbl">Sort field</span>
                <input v-model="sortField" type="text" placeholder="_id" />
              </label>
              <label class="chk">
                <input v-model="sortDesc" type="checkbox" />
                <span>Убывание</span>
              </label>
            </div>
          </div>

          <div class="doc-list">
            <div v-for="doc in displayDocs" :key="String(doc._id)" class="doc-card">
              <div class="doc-head">
                <code class="doc-id">{{ doc._id }}</code>
                <div class="doc-actions">
                  <button type="button" class="link" @click="openEdit(doc)">Изменить</button>
                  <button type="button" class="link danger" @click="removeDoc(doc._id)">Удалить</button>
                </div>
              </div>
              <pre class="doc-json">{{ formatJSON(doc) }}</pre>
            </div>
            <p v-if="displayDocs.length === 0" class="muted pad">Нет документов по запросу.</p>
          </div>
        </div>

        <div v-else class="panel">
          <div class="toolbar">
            <input v-model="newIndexField" class="idx-input" placeholder="Поле индекса (например name)" />
            <button class="btn primary" type="button" @click="addIndex">Создать индекс</button>
            <button class="btn" type="button" @click="refreshIndexes">Обновить</button>
          </div>
          <ul class="idx-list">
            <li v-for="f in displayIndexes" :key="f">
              <code>{{ f }}</code>
              <button type="button" class="link danger" @click="removeIndex(f)">Удалить</button>
            </li>
            <li v-if="displayIndexes.length === 0" class="muted">Индексов пока нет.</li>
          </ul>
        </div>
      </div>

      <div v-else class="content">
        <div class="panel">
          <div class="toolbar">
            <input
              v-model="kvKeyInput"
              class="idx-input"
              placeholder="ключ"
              @keyup.enter="kvGetValue"
            />
            <button class="btn" type="button" @click="kvGetValue">Получить</button>
            <button class="btn primary" type="button" @click="showKVSet = true">Записать</button>
            <button class="btn" type="button" @click="refreshKV">Обновить</button>
            <button class="btn danger-btn" type="button" @click="kvFlushAll">Очистить всё</button>
          </div>

          <div class="query-grid">
            <label>
              <span class="lbl">Фильтр по префиксу</span>
              <input v-model="kvPrefix" type="text" placeholder="session:" @keyup.enter="refreshKV" />
            </label>
            <div class="query-opts">
              <button class="btn-sm primary" type="button" @click="refreshKV">Применить префикс</button>
            </div>
          </div>

          <div v-if="kvSelected" class="doc-card kv-value-card">
            <div class="doc-head">
              <code class="doc-id">{{ kvSelected.key }}</code>
              <div class="doc-actions">
                <button type="button" class="link" @click="kvEditSelected">Изменить</button>
                <button type="button" class="link danger" @click="kvDeleteKey(kvSelected.key)">Удалить</button>
              </div>
            </div>
            <pre class="doc-json">{{ kvSelected.value }}</pre>
          </div>

          <div class="kv-key-list">
            <div v-for="k in kvKeys" :key="k" class="kv-key-row" @click="kvOpenKey(k)">
              <span class="kv-key-icon">⚡</span>
              <code class="kv-key-name">{{ k }}</code>
              <button type="button" class="link danger" @click.stop="kvDeleteKey(k)">×</button>
            </div>
            <p v-if="kvKeys.length === 0" class="muted pad">Ключей нет.</p>
          </div>
        </div>
      </div>
    </main>

    <div v-if="showKVSet" class="modal-overlay" @click.self="showKVSet = false">
      <div class="modal">
        <h3>Записать ключ</h3>
        <label class="lbl">Ключ</label>
        <input v-model="kvSetKey" class="modal-input" type="text" placeholder="session:42" />
        <label class="lbl">Значение</label>
        <textarea v-model="kvSetValue" rows="6" placeholder="значение" />
        <label class="lbl">TTL (секунды, 0 = без срока)</label>
        <input v-model.number="kvSetTTL" class="modal-input" type="number" min="0" />
        <div class="modal-actions">
          <button type="button" class="btn" @click="showKVSet = false">Отмена</button>
          <button type="button" class="btn primary" @click="kvSaveValue">Сохранить</button>
        </div>
      </div>
    </div>

    <div v-if="showInsert" class="modal-overlay" @click.self="showInsert = false">
      <div class="modal">
        <h3>Добавить документ</h3>
        <textarea v-model="insertJSON" rows="10" placeholder='{ "name": "example" }' />
        <div class="modal-actions">
          <button type="button" class="btn" @click="showInsert = false">Отмена</button>
          <button type="button" class="btn primary" @click="doInsert">Вставить</button>
        </div>
      </div>
    </div>

    <div v-if="editDoc" class="modal-overlay" @click.self="editDoc = null">
      <div class="modal wide">
        <h3>Изменить документ</h3>
        <textarea v-model="editJSON" rows="14" />
        <div class="modal-actions">
          <button type="button" class="btn" @click="editDoc = null">Отмена</button>
          <button type="button" class="btn primary" @click="saveEdit">Сохранить</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import {
  ListCollections,
  CreateCollection,
  DropCollection,
  InsertDocument,
  DeleteDocument,
  FindDocumentsQuery,
  CollectionStats,
  ListIndexes,
  CreateIndex,
  DropIndex,
  UpdateDocument,
  GetServerURL,
  SetServerURL,
  GetAPIKey,
  SetAPIKey,
  PingServer,
  KVKeys,
  KVGet,
  KVSet,
  KVDelete,
  KVStats,
  KVFlush,
} from "../wailsjs/go/main/App";

const serverURLInput = ref("");
const apiKeyInput = ref("");
const displayURL = ref<string>("");
const collections = ref<string[]>([]);
const selectedCollection = ref("");
const newCollName = ref("");
const documents = ref<Record<string, unknown>[]>([]);
const displayDocs = computed(() => documents.value ?? []);
const filterJSON = ref("{}");
const queryLimit = ref(0);
const querySkip = ref(0);
const sortField = ref("");
const sortDesc = ref(false);
const stats = ref<{ count: number; storageSize: number } | null>(null);
const activeTab = ref("Документы");
const indexes = ref<string[]>([]);
const displayIndexes = computed(() => indexes.value ?? []);
const newIndexField = ref("");
const showInsert = ref(false);
const insertJSON = ref('{\n  "name": "example"\n}');
const editDoc = ref<Record<string, unknown> | null>(null);
const editJSON = ref("");

// In-memory KV store state
const mode = ref<"collections" | "kv">("collections");
const kvKeys = ref<string[]>([]);
const kvPrefix = ref("");
const kvKeyInput = ref("");
const kvSelected = ref<{ key: string; value: string } | null>(null);
const kvStats = ref<Record<string, unknown> | null>(null);
const showKVSet = ref(false);
const kvSetKey = ref("");
const kvSetValue = ref("");
const kvSetTTL = ref(0);

onMounted(async () => {
  try {
    const u = await GetServerURL();
    serverURLInput.value = u;
    displayURL.value = u;
    apiKeyInput.value = await GetAPIKey();
  } catch {
    serverURLInput.value = "http://127.0.0.1:8080";
    displayURL.value = serverURLInput.value;
  }
  await refreshCollections();
});

async function applyServerURL() {
  try {
    await SetServerURL(serverURLInput.value.trim());
    await SetAPIKey(apiKeyInput.value);
    displayURL.value = await GetServerURL();
    selectedCollection.value = "";
    await refreshCollections();
  } catch (e) {
    alert(String(e));
  }
}

async function testConnection() {
  try {
    await SetServerURL(serverURLInput.value.trim());
    await SetAPIKey(apiKeyInput.value);
    await PingServer();
    displayURL.value = await GetServerURL();
    alert("Сервер доступен");
  } catch (e) {
    alert("Нет соединения: " + e);
  }
}

watch(selectedCollection, async (name) => {
  if (!name) return;
  await loadStats();
  await refreshIndexes();
  await loadAll();
});

async function refreshCollections() {
  try {
    collections.value = await ListCollections();
  } catch (e) {
    console.error(e);
    alert(String(e));
  }
}

async function createCollection() {
  if (!newCollName.value.trim()) return;
  try {
    await CreateCollection(newCollName.value.trim());
    newCollName.value = "";
    await refreshCollections();
  } catch (e) {
    alert(String(e));
  }
}

async function dropCollection(name: string) {
  if (!confirm(`Удалить коллекцию «${name}»?`)) return;
  try {
    await DropCollection(name);
    if (selectedCollection.value === name) selectedCollection.value = "";
    await refreshCollections();
  } catch (e) {
    alert(String(e));
  }
}

function selectCollection(name: string) {
  selectedCollection.value = name;
}

async function loadStats() {
  if (!selectedCollection.value) return;
  try {
    const s = await CollectionStats(selectedCollection.value);
    stats.value = {
      count: Number(s.count),
      storageSize: Number(s.storageSize),
    };
  } catch {
    stats.value = null;
  }
}

async function refreshIndexes() {
  if (!selectedCollection.value) return;
  try {
    indexes.value = await ListIndexes(selectedCollection.value);
  } catch {
    indexes.value = [];
  }
}

async function addIndex() {
  if (!selectedCollection.value || !newIndexField.value.trim()) return;
  try {
    await CreateIndex(selectedCollection.value, newIndexField.value.trim());
    newIndexField.value = "";
    await refreshIndexes();
  } catch (e) {
    alert(String(e));
  }
}

async function removeIndex(field: string) {
  if (!selectedCollection.value) return;
  try {
    await DropIndex(selectedCollection.value, field);
    await refreshIndexes();
  } catch (e) {
    alert(String(e));
  }
}

function buildQuery(): Record<string, unknown> {
  let filter: Record<string, unknown> = {};
  try {
    const parsed = JSON.parse(filterJSON.value || "{}");
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      filter = parsed as Record<string, unknown>;
    }
  } catch {
    filter = {};
  }
  const q: Record<string, unknown> = { filter };
  if (queryLimit.value > 0) q.limit = queryLimit.value;
  if (querySkip.value > 0) q.skip = querySkip.value;
  if (sortField.value.trim()) {
    q.sort = { field: sortField.value.trim(), order: sortDesc.value ? -1 : 1 };
  }
  return q;
}

async function runQuery() {
  if (!selectedCollection.value) return;
  try {
    const rows = await FindDocumentsQuery(selectedCollection.value, buildQuery());
    documents.value = rows ?? [];
  } catch (e) {
    alert(String(e));
  }
}

async function loadAll() {
  filterJSON.value = "{}";
  queryLimit.value = 0;
  querySkip.value = 0;
  sortField.value = "";
  sortDesc.value = false;
  if (!selectedCollection.value) return;
  try {
    const rows = await FindDocumentsQuery(selectedCollection.value, {});
    documents.value = rows ?? [];
  } catch (e) {
    alert(String(e));
  }
}

async function doInsert() {
  if (!selectedCollection.value) return;
  let doc: Record<string, unknown>;
  try {
    doc = JSON.parse(insertJSON.value);
  } catch {
    alert("Неверный JSON");
    return;
  }
  try {
    await InsertDocument(selectedCollection.value, doc);
    showInsert.value = false;
    await loadStats();
    await runQuery();
  } catch (e) {
    alert(String(e));
  }
}

async function removeDoc(id: unknown) {
  if (!selectedCollection.value || id === undefined) return;
  try {
    await DeleteDocument(selectedCollection.value, String(id));
    await loadStats();
    await runQuery();
  } catch (e) {
    alert(String(e));
  }
}

function openEdit(doc: Record<string, unknown>) {
  editDoc.value = doc;
  editJSON.value = JSON.stringify(doc, null, 2);
}

async function saveEdit() {
  if (!selectedCollection.value || !editDoc.value) return;
  const id = editDoc.value._id;
  let upd: Record<string, unknown>;
  try {
    upd = JSON.parse(editJSON.value);
  } catch {
    alert("Неверный JSON");
    return;
  }
  try {
    await UpdateDocument(selectedCollection.value, String(id), upd);
    editDoc.value = null;
    await loadStats();
    await runQuery();
  } catch (e) {
    alert(String(e));
  }
}

function formatJSON(doc: Record<string, unknown>) {
  return JSON.stringify(doc, null, 2);
}

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

function exportDocs() {
  const blob = new Blob([JSON.stringify(documents.value ?? [], null, 2)], {
    type: "application/json",
  });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = `${selectedCollection.value || "export"}.json`;
  a.click();
  URL.revokeObjectURL(a.href);
}

// ---- In-memory KV store ----

async function switchToKV() {
  mode.value = "kv";
  await refreshKV();
}

async function loadKVStats() {
  try {
    kvStats.value = await KVStats();
  } catch {
    kvStats.value = null;
  }
}

async function refreshKV() {
  try {
    kvKeys.value = (await KVKeys(kvPrefix.value.trim())) ?? [];
    await loadKVStats();
  } catch (e) {
    alert(String(e));
  }
}

async function kvGetValue() {
  const key = kvKeyInput.value.trim();
  if (!key) return;
  try {
    const value = await KVGet(key);
    kvSelected.value = { key, value };
  } catch (e) {
    alert(String(e));
  }
}

async function kvOpenKey(key: string) {
  kvKeyInput.value = key;
  try {
    kvSelected.value = { key, value: await KVGet(key) };
  } catch (e) {
    alert(String(e));
  }
}

function kvEditSelected() {
  if (!kvSelected.value) return;
  kvSetKey.value = kvSelected.value.key;
  kvSetValue.value = kvSelected.value.value;
  kvSetTTL.value = 0;
  showKVSet.value = true;
}

async function kvSaveValue() {
  const key = kvSetKey.value.trim();
  if (!key) {
    alert("Ключ обязателен");
    return;
  }
  try {
    await KVSet(key, kvSetValue.value, Number(kvSetTTL.value) || 0);
    showKVSet.value = false;
    kvSelected.value = { key, value: kvSetValue.value };
    await refreshKV();
  } catch (e) {
    alert(String(e));
  }
}

async function kvDeleteKey(key: string) {
  if (!confirm(`Удалить ключ «${key}»?`)) return;
  try {
    await KVDelete(key);
    if (kvSelected.value?.key === key) kvSelected.value = null;
    await refreshKV();
  } catch (e) {
    alert(String(e));
  }
}

async function kvFlushAll() {
  if (!confirm("Удалить ВСЕ ключи из in-memory хранилища?")) return;
  try {
    await KVFlush();
    kvSelected.value = null;
    await refreshKV();
  } catch (e) {
    alert(String(e));
  }
}
</script>

<style scoped>
.compass {
  display: flex;
  height: 100vh;
  background: var(--bg-app);
  color: var(--text);
}

.sidebar {
  width: 280px;
  background: var(--bg-panel);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  padding: 16px 12px;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 20px;
  padding: 0 4px;
}

.logo {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  object-fit: cover;
  flex-shrink: 0;
  display: block;
}

.brand-title {
  font-weight: 600;
  font-size: 15px;
}

.brand-sub {
  font-size: 11px;
  color: var(--text-muted);
}

.sidebar-section {
  margin-bottom: 16px;
}

.sidebar-section.grow {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.sidebar-label {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
  margin-bottom: 8px;
}

.conn-pill {
  font-size: 12px;
  padding: 8px 10px;
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--text-muted);
}

.conn-pill.active {
  border-color: var(--accent);
  color: var(--text);
}

.conn-box {
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 12px;
}

.conn-url {
  width: 100%;
  padding: 8px 10px;
  font-size: 12px;
  margin-bottom: 8px;
}

.conn-actions {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.row-between {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.icon-btn {
  background: transparent;
  border: none;
  color: var(--text-muted);
  font-size: 16px;
  padding: 4px 8px;
  border-radius: var(--radius);
}

.icon-btn:hover {
  background: var(--bg-elevated);
  color: var(--text);
}

.new-coll {
  display: flex;
  gap: 6px;
  margin-bottom: 10px;
}

.new-coll input {
  flex: 1;
  padding: 8px 10px;
  font-size: 13px;
}

.btn-sm {
  padding: 6px 10px;
  font-size: 12px;
  border-radius: var(--radius);
  border: 1px solid var(--border);
  background: var(--bg-elevated);
  color: var(--text);
}

.btn-sm.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
}

.btn-sm.primary:hover {
  background: var(--accent-hover);
}

.coll-list {
  list-style: none;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  flex: 1;
}

.coll-list li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: var(--radius);
  cursor: pointer;
  font-size: 13px;
  color: var(--text-muted);
}

.coll-list li:hover {
  background: var(--bg-elevated);
  color: var(--text);
}

.coll-list li.active {
  background: rgba(63, 163, 77, 0.15);
  color: var(--text);
  border: 1px solid rgba(63, 163, 77, 0.35);
}

.coll-icon {
  opacity: 0.7;
}

.coll-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.coll-del {
  opacity: 0;
  border: none;
  background: transparent;
  color: var(--danger);
  font-size: 18px;
  line-height: 1;
  padding: 0 4px;
}

.coll-list li:hover .coll-del {
  opacity: 1;
}

.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.topbar {
  padding: 12px 20px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  background: var(--bg-panel);
}

.breadcrumb {
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.crumb.dim {
  color: var(--text-muted);
}

.sep {
  color: var(--text-muted);
}

.tabs {
  display: flex;
  gap: 4px;
}

.tabs button {
  padding: 8px 14px;
  font-size: 13px;
  border: 1px solid transparent;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-muted);
}

.tabs button.on {
  background: var(--bg-elevated);
  border-color: var(--border);
  color: var(--text);
}

.empty-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
}

.empty-main h2 {
  margin: 0 0 8px;
  color: var(--text);
  font-weight: 500;
}

.content {
  flex: 1;
  overflow: auto;
  padding: 16px 20px 24px;
}

.panel {
  max-width: 1100px;
}

.stats-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  font-size: 13px;
}

.muted {
  color: var(--text-muted);
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}

.btn {
  padding: 8px 14px;
  font-size: 13px;
  border-radius: var(--radius);
  border: 1px solid var(--border);
  background: var(--bg-elevated);
  color: var(--text);
}

.btn:hover {
  border-color: var(--text-muted);
}

.btn.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
}

.btn.primary:hover {
  background: var(--accent-hover);
}

.query-grid {
  display: grid;
  grid-template-columns: 1fr 220px;
  gap: 16px;
  margin-bottom: 20px;
}

@media (max-width: 900px) {
  .query-grid {
    grid-template-columns: 1fr;
  }
}

.lbl {
  display: block;
  font-size: 11px;
  color: var(--text-muted);
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.query-grid textarea {
  width: 100%;
  padding: 10px;
  resize: vertical;
}

.query-opts {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.query-opts input[type="number"],
.query-opts input[type="text"] {
  width: 100%;
  padding: 8px 10px;
}

.chk {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-muted);
}

.doc-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.doc-card {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-panel);
  overflow: hidden;
}

.doc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border);
}

.doc-id {
  font-size: 12px;
  color: var(--focus);
}

.doc-actions {
  display: flex;
  gap: 12px;
}

.link {
  background: none;
  border: none;
  color: var(--focus);
  font-size: 12px;
  padding: 0;
}

.link.danger {
  color: var(--danger);
}

.doc-json {
  margin: 0;
  padding: 12px;
  font-size: 12px;
  line-height: 1.45;
  overflow-x: auto;
  font-family: var(--font-mono);
  color: var(--text-muted);
}

.pad {
  padding: 16px 0;
}

.idx-input {
  flex: 1;
  min-width: 160px;
  padding: 8px 10px;
}

.idx-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.idx-list li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  margin-bottom: 8px;
  background: var(--bg-panel);
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  padding: 24px;
}

.modal {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 20px;
  width: 100%;
  max-width: 480px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.45);
}

.modal.wide {
  max-width: 720px;
}

.modal h3 {
  margin: 0 0 12px;
  font-size: 16px;
  font-weight: 600;
}

.modal textarea {
  width: 100%;
  padding: 12px;
  margin-bottom: 16px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

/* ---- Mode switch + KV store ---- */
.mode-switch {
  display: flex;
  gap: 4px;
  margin-bottom: 16px;
  background: var(--bg-elevated);
  padding: 4px;
  border-radius: var(--radius);
  border: 1px solid var(--border);
}

.mode-switch button {
  flex: 1;
  padding: 8px 10px;
  font-size: 12px;
  border: none;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}

.mode-switch button.on {
  background: var(--accent);
  color: #fff;
}

.kv-stats-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text);
}

.danger-btn {
  color: var(--danger);
  border-color: var(--danger);
}

.danger-btn:hover {
  background: rgba(248, 81, 73, 0.12);
}

.kv-value-card {
  margin-bottom: 16px;
}

.kv-key-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.kv-key-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-panel);
  cursor: pointer;
}

.kv-key-row:hover {
  border-color: var(--accent);
  background: var(--bg-elevated);
}

.kv-key-icon {
  color: var(--accent);
}

.kv-key-name {
  flex: 1;
  font-size: 12px;
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.modal-input {
  width: 100%;
  padding: 8px 10px;
  margin-bottom: 12px;
}
</style>
