# Evelent DB client (`src-server/libs/db`)

Клиентская TypeScript-обвязка для работы с HTTP API базы данных Evelent.

## Быстрый старт

```ts
import { EvelentClient } from "./libs/db";

const db = EvelentClient.connect("http://localhost:3000");
```

Эквивалентно:

```ts
const db = new EvelentClient("http://localhost:3000");
```

## Структура клиента

- `db.collections` - операции с коллекциями.
- `db.collection(name)` - scoped-доступ к конкретной коллекции.
- `db.health` - health-check сервиса.
- `db.http` - низкоуровневый HTTP-клиент (обычно не нужен напрямую).

Пример:

```ts
const users = db.collection("users");

await users.documents.insertOne({ email: "user@example.com", age: 25 });
```

## Работа с коллекциями (`CollectionsApi`)

```ts
// список коллекций
const names = await db.collections.list();

// создание коллекции
await db.collections.create("users");

// удаление коллекции
await db.collections.drop("users");
```

## Работа с документами (`DocumentsApi`)

`const users = db.collection("users").documents;`

### Insert

```ts
const one = await users.insertOne({ email: "a@site.com", age: 20 });
// one._id

const many = await users.insertMany([
  { email: "b@site.com", age: 21 },
  { email: "c@site.com", age: 22 },
]);
// many.ids, many.inserted
```

### Read

```ts
const doc = await users.getById("document-id");

const found = await users.find({
  filter: { age: { $gte: 21 } },
  sort: { field: "age", order: -1 },
  limit: 20,
  skip: 0,
  projection: { email: 1, age: 1 },
});
```

### Update/Replace

```ts
await users.replaceById("document-id", {
  email: "updated@site.com",
  age: 30,
});
```

### Delete

```ts
await users.deleteById("document-id");
```

## Индексы (`IndexesApi`)

`const indexes = db.collection("users").indexes;`

```ts
await indexes.create("email");
const fields = await indexes.list();
await indexes.drop("email");
```

## Статистика коллекции (`StatsApi`)

`const stats = await db.collection("users").stats.get();`

Возвращает:

- `stats.count` - количество документов.
- `stats.storageSize` - размер хранилища (в байтах, если сервер отдает в байтах).

## Health-check (`HealthApi`)

```ts
const status = await db.health.ping();
```

Запрашивает `GET /health` и возвращает текстовый ответ сервера.

## Типы и запросы

Основные типы лежат в `types.ts` и реэкспортируются из `index.ts`:

- `Document` - JSON-объект с обязательным `_id`.
- `FindQuery` - параметры поиска.
- `Filter` / `FilterOperator` - фильтрация.
- `InsertOneResult` / `InsertManyResult`.
- `CollectionStats`.

Поддерживаемые операторы фильтра:

- `$eq`, `$ne`
- `$gt`, `$gte`
- `$lt`, `$lte`
- `$in`

## Обработка ошибок

При HTTP/сетевых ошибках бросается `EvelentError`:

```ts
import { EvelentError } from "./libs/db";

try {
  await db.collections.create("users");
} catch (error) {
  if (error instanceof EvelentError) {
    console.error(error.status, error.statusText, error.body);
  }
}
```

Поля ошибки:

- `status` - HTTP статус (или `0` для сетевой ошибки).
- `statusText` - текст статуса.
- `body` - тело ответа/дополнительные детали.

## Экспорты модуля

Из `src-server/libs/db/index.ts` доступны:

- `EvelentClient`, `CollectionScope`, `HttpClient`, `EvelentError`
- `CollectionsApi`, `DocumentsApi`, `IndexesApi`, `StatsApi`, `HealthApi`
- все типы из `types.ts`

## Ограничения и "кд" (по текущему серверу)

Ниже факты из серверной реализации `apps/evelentdb/database/server/`:

- Явного ограничения размера body для `insertOne` / `insertMany` на уровне кода нет.
- Встроенного rate limit / throttling / cooldown ("кд") тоже нет.
- Есть HTTP-таймауты сервера:
  - `http-read-header-timeout`: `10s`
  - `http-read-timeout`: `60s` (чтение всего запроса)
  - `http-idle-timeout`: `180s`
  - `http-write-timeout`: `0` (без таймаута)
- Лимит заголовков: `MaxHeaderBytes = 1 MiB`.
- Для `find` лимиты есть:
  - по умолчанию: `limit = 500`
  - максимум: `limit = 100000` (все больше обрезается до 100000)

Вывод: практически потолок на `insert` сейчас упирается в сеть, размер JSON, диск и таймаут `60s`, а не в hard limit в коде.

## Рекомендации по размеру `insert`

Так как жесткого лимита нет, лучше использовать операционные лимиты:

- `insertOne`: держать документ до `256 KB - 1 MB` (для стабильной latency).
- `insertMany`: отправлять батчами по `100-1000` документов.
- Целиться в payload батча до `5-20 MB` и держать p95 ответа существенно ниже `60s`.
- Если документы "тяжелые" (много полей/вложенности), уменьшать батч до `50-200`.
- Если сервер/диск под нагрузкой, переходить на более мелкие батчи и параллелизм `2-4` воркера вместо одного огромного запроса.

Если нужен строгий SLA, зафиксируйте лимиты в вашем приложении (например, `maxDocsPerBatch` и `maxPayloadBytes`) и валидируйте до отправки.

## Гайд по производительности

- Создавайте индексы на полях, по которым часто фильтруете/сортируете (`indexes.create("field")`).
- Не ставьте `find.limit` слишком большим без необходимости; начинайте с `100-1000`.
- Используйте `projection`, чтобы не тащить лишние поля.
- Для больших выборок делайте пагинацию (`skip + limit` или `cursor`).
- Следите за `stats.get()` (`count`, `storageSize`) и ростом размера коллекций.

## Практический паттерн: безопасная batch-загрузка

```ts
import { EvelentClient, JsonObject } from "./libs/db";

const db = EvelentClient.connect("http://localhost:8080");
const users = db.collection("users").documents;

async function insertInChunks(rows: JsonObject[], chunkSize = 500) {
  for (let i = 0; i < rows.length; i += chunkSize) {
    const chunk = rows.slice(i, i + chunkSize);
    await users.insertMany(chunk);
  }
}
```

Для high-load сценариев добавьте:

- ограниченный параллелизм (например, 2-4 одновременных чанка),
- retry с backoff на сетевые ошибки (`EvelentError` со `status = 0`),
- логирование времени каждого батча и размера payload.
