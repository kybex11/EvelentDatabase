# Производительность Evelent DB — понятный гайд

Цель: быстро понять, **что тормозит**, и **что крутить**.

## 1. Сначала измерь

```bash
cd server
go test ./internal/db -bench . -benchmem -count=1
```

Смотри stats коллекции:

```bash
curl http://127.0.0.1:8080/api/collections/users/stats
```

Ответ roughly:

```json
{
  "count": 10000,
  "storageSize": 1234567,
  "segments": 2,
  "indexes": ["age", "email"],
  "syncMode": "none",
  "docCache": { "hits": 900, "misses": 100, "hitRatio": 0.9, ... }
}
```

Если `hitRatio` низкий на горячих Get — данных больше, чем cache (8k / 64 MiB),
или ключи слишком «холодные».

---

## 2. Чеклист «сделай это первым»

### Документы пишутся медленно?

1. **Не используй `every_write`**, пока не нужна железная durability → `none` или `every_sec`.
2. Пиши пачками: `POST .../docs/batch`, а не по одному.
3. Убедись, что не делаешь Find без индекса на больших коллекциях.
4. После многих Update/Delete — дождись compaction (или вызови Compact).

### Документы читаются медленно?

1. Читай по `_id` — это быстрый путь (cache + FD).
2. Для фильтров — **создай индекс** на поле.
3. Для диапазонов возраста/даты — индекс + `$gte`/`$lt` (range path).
4. Пагинация: `after` / `nextCursor`, **не** огромный `skip`.

### KV тормозит?

1. Используй `MGET`/`MSET`/`MDEL` вместо тысяч одиночных запросов.
2. Ограничь размер: иначе всё в RAM и GC страдает.
3. Не держи гигантские lists — `LPUSH` копирует (copy-on-write).

---

## 3. Индексы: equality и range

```json
// equality — O(1)
{ "filter": { "email": "a@b.c" } }

// range — использует отсортированный индекс, если он есть
{ "filter": { "age": { "$gte": 18, "$lt": 65 } } }
```

Без индекса оба случая = полный скан + decrypt всех документов.

Создать:

```bash
curl -X POST http://127.0.0.1:8080/api/collections/users/indexes \
  -d '{"field":"age"}'
```

Индексы живут в RAM. Слишком много уникальных значений × огромная коллекция =
много памяти.

---

## 4. Durability vs скорость

| `-sync-mode` | Скорость записи | Риск потери при crash |
|--------------|-----------------|------------------------|
| `none` | ★★★★★ | до ~1с meta + несинкнутые сегменты |
| `every_sec` | ★★★★☆ | обычно ≤1с |
| `every_write` | ★☆☆☆☆ | минимальный для payload |

На shutdown (`Ctrl+C`) всё равно flush meta + close сегментов.

---

## 5. Compaction — зачем

Update/Delete оставляют «дыры» в `.seg` файлах. Диск растёт, хотя живых
документов меньше.

Автоматика: если live < 50% размера сегментов и суммарно > 8 MiB.

После compaction `storageSize` обычно падает, Get не меняется по смыслу.

---

## 6. Что внутри уже оптимизировано

- Batched flush docindex/indexes (~1с)
- Binary docindex (`EDIX` + gob), JSON legacy читается
- FD-кэш сегментов + buffer pool на чтение
- Hot document LRU (clone на выдачу — безопасно мутировать ответ)
- Reuse AES-GCM
- Sharded LRU + striped RMW locks в KV
- Range secondary indexes
- Быстрый `valuesEqual` без `reflect.DeepEqual` на скалярах

---

## 7. Чего пока нет (и не жди чудес)

- Настоящий B-tree / WiredTiger / LSM
- Транзакции / MVCC
- Распределённый шардинг между машинами
- Query planner уровня Mongo SBE

Это **встроенная** БД для одного хоста. Для «максимума» на одной машине:
индексы + batch + sync-mode + compaction + горячий кэш.

---

## 8. Типичные рецепты

**Лог событий, много Insert, редкий Find по id**

```bash
-sync-mode none
# индекс не обязателен, если почти нет фильтрации
# периодически Compact если есть Update
```

**Пользователи, поиск по email/age**

```bash
-sync-mode every_sec
# индексы на email и age
# Find только через индексированные поля
```

**Сессии / rate-limit counters**

```text
используй KV (TTL), не document store
```

---

## 9. Дальнейшее чтение

- [architecture.ru.md](architecture.ru.md) — устройство
- [large-data.md](large-data.md) — большие объёмы
- [configuration.md](configuration.md) — все флаги
