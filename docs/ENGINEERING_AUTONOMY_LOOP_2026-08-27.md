# KeyboardAZ — контур циклической разработки, тестирования и контролируемого автоисправления

Дата: 27 августа 2026  
Источник задач: `docs/PARETO_IMPLEMENTATION_PLAN_2026-08-26.md`  
Machine-readable execution state: `engineering/loop.json`  
Многомерное пространство граничных случаев: `engineering/edge-space.json`

## 1. Назначение

Этот контур превращает главный Pareto-план из документа в исполняемый процесс разработки. Его задача — не просто «писать код и запускать тесты», а системно выполнять следующий незаблокированный этап плана, заранее формализовать пространство ошибок, доказывать корректность несколькими независимыми методами и ограничивать автоматические исправления.

Контур рассчитан на работу coding-agent/LLM, человека или их комбинации. Он не доверяет самому агенту решение о том, что работа «готова»: готовность определяется внешними gates, mutation testing, архитектурными инвариантами и, когда требуется, физическим HIL.

Главная формула:

```text
PLAN
  -> SELECT NEXT WORK ITEM
  -> IMPACT MAP
  -> MULTIDIMENSIONAL EDGE SPACE
  -> TEST FIRST / REPRODUCE
  -> IMPLEMENT MINIMAL SLICE
  -> TARGETED TESTS
  -> STATIC + RACE
  -> FUZZ / PROPERTY
  -> MUTATION TEST-OF-TESTS
  -> FULL PLATFORM/FIRMWARE GATES
  -> DIFF / RISK REVIEW
  -> COMMIT + PUSH main
  -> UPDATE PLAN STATE
  -> NEXT ITERATION
```

Если любой доказательный gate красный, переход к commit запрещён.

---

## 2. Что считается источником истины

### План продукта

`docs/PARETO_IMPLEMENTATION_PLAN_2026-08-26.md`

Он определяет цель и порядок крупных этапов.

### Состояние исполнения

`engineering/loop.json`

Он хранит:

- stage ID;
- ссылку на heading главного плана;
- `done / in_progress / partial / blocked / pending`;
- зависимости;
- физические blocker'ы;
- набор обязательных edge-space сценариев;
- gate profiles;
- mutation thresholds;
- controlled-autofix budget.

Изменение статуса stage без соответствующего evidence запрещено.

### Фактический отчёт

`docs/PARETO_IMPLEMENTATION_PROGRESS_2026-08-26.md`

Он описывает уже реально работающий код и не может использоваться для объявления физических HIL/SLO, которых в действительности не измеряли.

### Edge-space

`engineering/edge-space.json`

Это не список случайных edge cases, а многомерная модель взаимодействий.

---

## 3. Состояния одной итерации

Каждая итерация проходит конечный автомат:

```text
PLANNED
  -> SCOPED
  -> TEST_DESIGNED
  -> IMPLEMENTING
  -> TARGETED_GREEN
  -> ADVERSARIAL_GREEN
  -> MUTATION_GREEN
  -> FULL_GREEN
  -> REVIEWED
  -> COMMITTED
  -> PLAN_SYNCED
```

Ошибка переводит работу не в начало, а в классифицированное repair-состояние:

```text
DETERMINISTIC_REPAIR
SEMANTIC_REPAIR
TEST_REPAIR
INFRA_RETRY
PHYSICAL_BLOCKED
HUMAN_REVIEW_REQUIRED
```

Это важно: одинаковая стратегия «попросить модель ещё раз исправить всё» запрещена.

---

## 4. Выбор следующей задачи

Команда:

```powershell
cd go-app
go run ./tools/devloop -cmd next
```

Алгоритм:

1. загрузить `engineering/loop.json`;
2. проверить schema и DAG зависимостей;
3. исключить `done`;
4. исключить stages с реальным `blocked_by`;
5. проверить зависимости;
6. выбрать минимальный `priority`;
7. вывести один work item.

Если P0/P1 этап заблокирован физической оснасткой, агент **не имеет права** придумывать HIL-результат. Он фиксирует blocker и берёт следующий независимый executable stage.

---

## 5. Impact map перед написанием кода

До изменения production-кода для work item фиксируются:

- какие packages/files являются владельцами поведения;
- какие public contracts меняются;
- какой hot path затрагивается;
- какие persistent/wire schemas затрагиваются;
- какие goroutines/resources меняют lifecycle;
- какие failure modes появляются;
- rollback point;
- какие existing tests должны упасть при намеренном нарушении нового требования.

Правило: если изменение нельзя описать как ограниченный impact map, оно слишком большое для одной итерации.

Максимально предпочтительный slice — один архитектурный контракт или одна observable capability.

---

# 6. Многомерное пространство граничных случаев

## 6.1 Почему не достаточно списка «edge cases»

Большинство сложных дефектов KeyboardAZ появляется не на одной границе, а на пересечении факторов:

```text
HID
× reconnect
× sequence wrap
× queue pressure
× shutdown
```

или:

```text
same-scan thumb+main
× RU
× Shift+Rare
× debounce boundary
```

Поэтому `engineering/edge-space.json` моделирует поведение по независимым dimensions.

Текущие измерения включают:

- transport;
- transport lifecycle state;
- semantic event type;
- sequence state;
- timestamp state;
- language;
- modifier mask;
- physical input pattern;
- debounce boundary;
- queue pressure;
- concurrency state;
- USB topology;
- identity state;
- report shape;
- SendInput result;
- configuration state;
- workspace state;
- process state;
- load profile;
- injected fault.

Полный Cartesian product намеренно не выполняется: он огромен и содержит много бессмысленных комбинаций.

Используется layered coverage.

### Level A — 1D boundaries

Для каждого изменённого dimension проверить:

- minimum;
- maximum;
- exact boundary;
- just below;
- just above;
- invalid/reserved;
- empty/zero/null, где применимо;
- wrap-around, где применимо.

### Level B — pairwise

Все изменённые dimensions должны быть покрыты попарно с непосредственно связанными dimensions.

`devloop -cmd edge-report` считает размер pairwise tuple-space и не позволяет тихо удалить dimension/scenario из manifest.

### Level C — 3-way critical interactions

Transport, lifecycle, sequence, identity, debounce и concurrency требуют explicit 3-way сценариев.

Примеры:

```text
hid-v3 × hot-unplug × simultaneous-close-read
cdc-v2 × renumbered-COM × exact VID/PID/serial
stroke × sequence wrap × timestamp wrap
macro-delay × near-capacity queue × realtime stroke
```

### Level D — named catastrophic 4-way scenarios

Для известных особо опасных пересечений допускается strength=4, например:

```text
transport × hot-unplug × shutdown race × queue pressure
physical input × modifier conflict × language × debounce boundary
```

### Level E — fuzz

Fuzzing исследует пространство за пределами вручную выбранных combinations, но не заменяет deterministic critical scenarios.

---

## 6.2 Invariant-driven testing

Сценарий описывает не только input, но и invariant.

Примеры:

- malformed report никогда не создаёт valid semantic event;
- ambiguous identity никогда не выбирается автоматически;
- uint32 wrap не считается packet loss;
- appcore остаётся единственным semantic source of truth;
- physical realtime action не дропается из-за macro delay;
- один physical HID event не может породить два SendInput trace;
- HIL instrumentation никогда не хранит typed content;
- migration never-overwrite;
- shutdown не запускает новый reconnect после terminal Close.

Тест без проверяемого invariant не считается edge-space coverage.

---

# 7. Test pyramid контура

Для каждого изменения применяются уровни снизу вверх.

## T0 — compile/schema

- JSON schema/manifest validation;
- `gofmt`;
- build tags;
- generated/wire layout compile assertions.

## T1 — unit

Чистые функции, state transitions, parsers, resolver, config transactions.

## T2 — property/table tests

Boundary tables, algebraic invariants, round trips, idempotency.

## T3 — race/concurrency

`go test -race` на изменённых ownership/lifecycle boundaries.

## T4 — fuzz

Native Go coverage-guided fuzzing минимум для parser/wire/config input surface.

В контур уже включены:

- `FuzzDecodeV3`;
- `FuzzParseCompactFormat`.

PR/push engineering workflow запускает bounded fuzz sessions; найденный corpus должен быть закоммичен как regression seed, если он воспроизводит production defect.

## T5 — mutation test-of-tests

Проверяет не production-код, а способность тестов обнаруживать правдоподобно сломанный production-код.

## T6 — platform/integration

Linux pure core + Go 1.27 compatibility + Windows application + native firmware + PlatformIO CDC/HID builds.

## T7 — physical HIL

Только для требований, которые нельзя доказать симуляцией:

- физический bounce;
- USB reconnect cycles;
- CDC-vs-HID E2E;
- 100k switch cycle correctness;
- fixture T0/T4.

CI/mock не имеет права заменить T7.

---

# 8. Test-of-tests через mutation testing

## 8.1 Инструмент

Pinned:

```text
github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
```

Конфигурация:

`go-app/.gremlins.yaml`

Включены mutations:

- arithmetic;
- conditional boundary;
- conditional negation;
- increment/decrement;
- assignment inversion;
- bitwise inversion;
- bitwise assignment inversion;
- negative inversion;
- logical inversion;
- loop-control inversion;
- self-assignment removal.

Платформенно-зависимый GUI/handler код исключён из Linux mutation run; его защищает Windows lane и отдельные tests.

## 8.2 Почему mutation запускается после green baseline

Порядок строго такой:

```text
green original tests
-> mutate one behavior
-> run tests
-> classify mutant
```

Если исходные tests уже красные, mutation result не имеет смысла.

## 8.3 Интерпретация

- `KILLED`: тесты доказали, что mutation нарушает contract;
- `LIVED`: тесты прошли на сломанной версии — test gap;
- `NOT COVERED`: production ветка не покрыта;
- `TIMED OUT`: mutation привела к pathological behavior; требуется отдельный deterministic timeout/deadlock test;
- `NOT VIABLE`: mutation не компилируется; не увеличивает test confidence.

## 8.4 Порог

Обычная mutation surface:

- efficacy ≥ 80%;
- mutant coverage ≥ 75%.

Critical algorithms:

- efficacy ≥ 85%;
- mutant coverage ≥ 80%;
- survived mutation в изменённом critical behavior рассматривается как blocking defect тестов.

`tools/devloop mutation-check` дополнительно проверяет, что JSON существует и `mutants_total > 0`. Поэтому ситуация «Gremlins вообще не запустился, а pipeline принял 0%/пустой output» не может пройти как успешный test-of-tests.

## 8.5 Что делать с survived mutant

Порядок ремонта:

1. понять изменённую semantics mutation;
2. найти invariant, который должен её убивать;
3. добавить минимальный regression test;
4. убедиться, что test падает на mutant;
5. убедиться, что test проходит на original;
6. повторить mutation;
7. только после этого продолжать stage.

Запрещено менять production logic только для того, чтобы поднять mutation score, если mutation эквивалентна исходному поведению. Equivalent mutant должен быть явно задокументирован/исключён узко, а не глобальным suppress.

---

# 9. Контролируемое автоисправление

Автоисправление делится на классы.

## Class S0 — deterministic safe

Разрешено автоматически без semantic review:

- `gofmt`.

Команда:

```powershell
go run ./tools/devloop -cmd safe-fix
```

После неё всё равно обязательны tests.

## Class S1 — bounded semantic repair

Coding-agent может исправлять production-код автоматически только если одновременно выполнены условия:

- failure воспроизводится;
- scope находится в allowlist;
- patch ≤ 8 файлов;
- patch ≤ 600 changed lines;
- не затрагивает workflow/policy/toolchain manifests;
- исправление не ослабляет test/gate;
- добавлен regression test для нового класса ошибки, если его не было;
- targeted tests + race снова зелёные.

Проверка patch budget:

```powershell
go run ./tools/devloop -cmd verify-diff -base <known-good-commit>
```

## Class S2 — policy-sensitive

Автоматически изменять запрещено:

- `.github/workflows/`;
- `engineering/` policy;
- `platformio.ini`;
- `go.mod/go.sum`;
- security boundary;
- persistent schema migration;
- release/signing policy.

Такие изменения являются нормальной реализацией stage, но не могут возникнуть как «автофикс упавшего теста».

## Class S3 — physical/safety decision

Никогда не автоисправлять на основании simulation:

- debounce default;
- HID production promotion;
- physical SLO;
- reconnect success SLO;
- switch reliability.

Для них требуется физический evidence.

---

# 10. Repair budget и защита от бесконечного цикла

`engineering/loop.json` фиксирует:

```text
max_semantic_repair_attempts = 3
max_infrastructure_retries = 2
```

После трёх semantic repairs одной и той же failure signature:

```text
STOP -> HUMAN_REVIEW_REQUIRED
```

После двух infrastructure retries:

- не менять production code;
- сохранить exact failure/log/seed;
- классифицировать runner/toolchain/network failure;
- если воспроизводится локально — это уже semantic/test failure.

Нельзя бесконечно re-run'ить flaky test до зелёного.

---

# 11. Failure classification

## Formatting

Автофикс S0 → tests.

## Compile error

Исправляется минимально в изменённом contract; нельзя удалять типизацию/gate.

## Unit/property failure

Ищется violated invariant. Если test неверен — это должно быть доказано через contract, а не просто переписано под текущий output.

## Race

Считать production correctness defect. Запрещено решать `time.Sleep` или увеличением timeout без доказательства ownership/happens-before.

## Fuzz failure

Сохранить seed. Создать deterministic regression test. Затем исправить.

## Mutation survived

Test defect, пока не доказана equivalence mutation.

## Mutation timeout

Добавить bounded termination/deadlock contract.

## Platform-only failure

Не маскировать build tag'ом без архитектурного основания. Исправлять на целевой платформе.

## Physical blocker

Stage остаётся blocked. Не генерировать synthetic evidence.

---

# 12. Commit/push protocol

Проект сейчас развивается прямыми атомарными commits в `main`. Для каждой завершённой итерации:

1. work item ограничен;
2. targeted tests зелёные;
3. relevant edge scenarios зелёные;
4. fuzz/property зелёные;
5. mutation обязателен для изменённой critical logic/test contract;
6. полный applicable CI зелёный;
7. `git diff --check`;
8. architecture fitness tests зелёные;
9. progress/loop state синхронизированы;
10. один атомарный commit;
11. push `main`;
12. следующий work item выбирается **только после успешного push**.

Если изменение крупнее одного проверяемого slice, оно делится на несколько commits. «Большой финальный commit после десяти невалидированных подэтапов» запрещён.

---

# 13. Definition of Done новой версии

Существующие 6 вопросов главного плана сохраняются и расширяются.

Для каждого изменения агент обязан ответить:

1. Какая stage главного плана выполняется?
2. Какой observable contract изменён?
3. Какой impact map?
4. Какие dimensions edge-space затронуты?
5. Какие pairwise/3-way/4-way scenarios проверены?
6. Какой test сначала воспроизводил дефект/требование?
7. Что показал race detector?
8. Какой fuzz target покрывает входную поверхность?
9. Какие mutants mutation suite убивает для этого behavior?
10. Есть ли survived/not-covered mutants?
11. Какие platform/firmware gates выполнены?
12. Нужен ли physical HIL?
13. Есть ли численное before/after?
14. Как выполнить rollback?
15. Обновлены ли plan state/progress?

Если критический пункт неприменим, указывается причина. Пустое «N/A» без причины не принимается.

---

# 14. Команды оператора/агента

Из `go-app`:

```powershell
# Проверить сам контур
go run ./tools/devloop -cmd validate

# Получить следующую незаблокированную stage
go run ./tools/devloop -cmd next

# Получить размер multidimensional test space
go run ./tools/devloop -cmd edge-report

# Быстрый deterministic/race gate
go run ./tools/devloop -cmd gate -gate fast

# Безопасный deterministic autofix
go run ./tools/devloop -cmd safe-fix

# Проверить bounded auto-repair patch
go run ./tools/devloop -cmd verify-diff -base HEAD~1

# Проверить machine-readable mutation result
go run ./tools/devloop -cmd mutation-check -mutation-report mutation-report.json
```

Mutation run:

```powershell
go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
gremlins unleash --config=.gremlins.yaml --output=mutation-report.json
go run ./tools/devloop -cmd mutation-check -mutation-report mutation-report.json
```

Critical changed behavior:

```powershell
go run ./tools/devloop -cmd mutation-check -critical -mutation-report mutation-report.json
```

---

# 15. GitHub Actions

`.github/workflows/engineering-loop.yml` имеет три независимых контура.

## plan-contract

На изменениях engineering contract:

- форматирование devloop;
- race tests самого orchestrator;
- schema/DAG validation;
- selection следующей stage;
- edge-space audit;
- artifacts `engineering-next.json` и `edge-space-report.json`.

## bounded-fuzz

Короткие deterministic-budget fuzz sessions для protocol v3 и CDC parser.

Цель push/PR fuzz — не «найти все баги за 20 секунд», а гарантировать, что fuzz targets компилируются, corpus остаётся валидным и свежие изменения немедленно проходят coverage-guided adversarial input.

Длинные fuzz campaigns могут запускаться отдельно.

## mutation-test-of-tests

Запускается weekly или вручную, потому что mutation значительно дороже обычных tests.

Порядок:

1. green baseline;
2. install pinned Gremlins;
3. mutation run;
4. обязательный непустой JSON;
5. devloop mutation policy check;
6. artifact с survived/not-covered mutants.

Это намеренно отдельный gate: Gremlins отмечает, что mutation runs на средних/больших modules могут быть длительными, поэтому выполнять полный mutation на каждый push нерационально.

---

# 16. Самодокументирование цикла

После успешной итерации обновлять:

- `engineering/loop.json` — machine state;
- `docs/PARETO_IMPLEMENTATION_PROGRESS_2026-08-26.md` — factual state;
- при изменении contracts — соответствующий protocol/architecture doc;
- benchmark/HIL/mutation artifact — не переписывать руками.

При контекстном сжатии coding-agent должен сохранять минимум:

```text
CURRENT_STAGE
LAST_GREEN_COMMIT
FILES_CHANGED
CONTRACTS_ADDED
TESTS_ADDED
EDGE_SCENARIOS_COVERED
MUTATION_STATUS
CI_STATUS
PHYSICAL_BLOCKERS
NEXT_ATOMIC_ACTION
```

Это позволяет продолжить цикл после compaction без повторного аудита всего репозитория.

---

# 17. Принцип controlled autonomy

Контур считается передовым не потому, что «агенту разрешено всё», а потому что автоматизация имеет чёткие пределы.

Агенту разрешено:

- выбирать следующий executable work item из утверждённого плана;
- писать минимальный slice;
- добавлять tests;
- применять безопасный deterministic fix;
- делать до трёх bounded semantic repairs;
- пушить atomic green commit в `main` в рамках принятого workflow проекта.

Агенту запрещено:

- ослаблять gate ради зелёного результата;
- удалять failing test без доказательства изменения requirements;
- переписывать mutation policy как autofix;
- придумывать HIL evidence;
- превращать flaky failure в бесконечные reruns;
- расширять scope исправления за patch budget без новой итерации;
- смешивать независимые архитектурные изменения в один repair;
- объявлять SLO достигнутым без измерения.

Именно эти ограничения делают auto-repair контролируемым, а цикл — воспроизводимым.
