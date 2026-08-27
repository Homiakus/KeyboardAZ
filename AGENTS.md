# KeyboardAZ engineering agent contract

Этот файл обязателен для любого coding-agent, который изменяет репозиторий.

## 1. Главный источник задач

Не придумывать собственную дорожную карту вместо утверждённой.

1. Прочитать `docs/PARETO_IMPLEMENTATION_PLAN_2026-08-26.md`.
2. Прочитать `engineering/loop.json`.
3. Выполнить из `go-app`:

```text
go run ./tools/devloop -cmd validate
go run ./tools/devloop -cmd next
```

4. Работать только над выданным незаблокированным stage либо над блокирующим defect, который обнаружил gate текущего stage.
5. Физически заблокированный HIL-stage не заменять simulation/mock результатом. Перейти к следующему независимому executable stage.

## 2. Один цикл = один доказуемый slice

Перед изменением кода сформировать:

```text
CURRENT_STAGE
OBSERVABLE_CONTRACT
IMPACT_MAP
FILES_OWNING_BEHAVIOR
EDGE_DIMENSIONS
NAMED_EDGE_SCENARIOS
ROLLBACK_POINT
TARGETED_TESTS
PHYSICAL_BLOCKERS
```

Slice должен быть минимальным: один архитектурный контракт или одна observable capability.

## 3. Обязательный порядок

```text
reproduce / test-design
-> implementation
-> targeted tests
-> race / vet
-> deterministic edge scenarios
-> fuzz/property when input/state surface changed
-> mutation test-of-tests when critical behavior/tests changed
-> full applicable platform/firmware gates
-> diff/risk review
-> progress + loop-state sync
-> atomic commit
-> push main
-> select next stage
```

Нельзя писать несколько крупных непроверенных этапов и тестировать их только в конце.

## 4. Edge-space

Использовать `engineering/edge-space.json`.

Минимум:

- 1D exact boundaries для изменённого поведения;
- pairwise с непосредственно связанными dimensions;
- 3-way для transport/lifecycle/identity/debounce/concurrency;
- named 4-way catastrophic scenarios, если stage их указывает;
- fuzzing только как дополнение, а не замена deterministic scenarios.

Каждый test должен проверять invariant, а не только отсутствие panic.

## 5. Test-of-tests

Pinned mutation tool:

```text
github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
```

Перед mutation исходные tests обязаны быть зелёными.

`LIVED` означает test gap, пока не доказана эквивалентность mutation. Для test gap:

1. определить нарушенный invariant;
2. добавить regression test;
3. доказать, что test убивает mutant;
4. повторить mutation;
5. production code не менять ради score, если mutation эквивалентна.

## 6. Controlled autofix

Разрешённый deterministic autofix:

```text
go run ./tools/devloop -cmd safe-fix
```

В настоящее время это только `gofmt`.

Semantic auto-repair:

- максимум 3 попытки на одну failure signature;
- максимум 8 файлов / 600 changed lines;
- проверить `go run ./tools/devloop -cmd verify-diff -base <known-green-commit>`;
- нельзя автоисправлением менять `.github/workflows/`, `engineering/`, `platformio.ini`, `go.mod`, `go.sum`;
- нельзя ослаблять test, timeout, threshold или architecture guard ради зелёного результата.

После трёх неуспешных semantic repairs: `HUMAN_REVIEW_REQUIRED`.

Infrastructure failure разрешено повторить максимум два раза без изменения production code.

## 7. Запреты

Нельзя:

- объявлять HIL/SLO выполненным без физического evidence;
- делать HID default до controlled CDC-v2/HID-v3 A/B gate;
- менять debounce default без physical bounce dataset;
- удалять failing test без доказанного изменения requirement;
- маскировать race через sleep/увеличение timeout;
- бесконечно rerun flaky test до случайного green;
- сохранять typed text/key content в latency trace;
- выбирать первый COM/HID при ambiguity;
- возвращать transport-specific types внутрь application semantics;
- смешивать несвязанные архитектурные изменения в auto-repair.

## 8. Definition of Done перед push

Проверить:

- stage главного плана известен;
- observable contract описан;
- edge scenarios покрыты;
- targeted tests зелёные;
- race/vet зелёные;
- fuzz regression seeds сохранены при найденном defect;
- mutation status известен для critical logic;
- applicable Windows/Linux/firmware gates зелёные;
- physical blocker явно отмечен, если есть;
- rollback понятен;
- progress/loop state не противоречат коду;
- commit атомарный.

После успешного push снова выполнить `devloop -cmd next`.

Полное описание: `docs/ENGINEERING_AUTONOMY_LOOP_2026-08-27.md`.
