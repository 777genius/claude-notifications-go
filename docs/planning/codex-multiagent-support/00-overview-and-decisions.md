# Мульти-агентный пивот + Codex support: полный контекст и обоснование решений

## 0. Что произошло на этой ветке

`feat/codex-notify` начиналась как узкий проект «добавить уведомления для Codex CLI» — был написан 5-фазный план (`01-phase0-groundwork.md` … `05-phase4-docs-release.md`) и прогнан через ~30 раундов критики. На середине скоуп сменился: масштабировать сразу на много агентов (Claude/Codex/Cursor/OpenCode/CodeBuddy), не переписывая существующую логику дважды — см. `06-multiagent-pivot.md`. Всё это планирование до сих пор жило только в рабочей scratchpad-директории — никогда не коммитилось. Ветка содержала 0 коммитов относительно `main` до этого PR.

Это уже один раз аукнулось: в соседнем репозитории `plugin-kit-ai` в этом же цикле планирования был написан и отревьюен Codex hooks decode-код, который остался только в незакоммиченном git worktree — при следующей попытке его использовать выяснилось, что worktree физически удалён с диска, а сам код нигде не сохранился (проверено: все висячие коммиты, все локальные и remote-ветки, стеш — ничего релевантного). Код придётся писать заново.

**Вывод**: прежде чем писать код, планирование фиксируется реальными коммитами и PR — чтобы работа была видна и защищена от повторной потери, и чтобы можно было получить фидбек по архитектуре до написания кода. Этот PR — только документы, ни одной строчки кода.

## 1. Почему мульти-агентный пивот, а не узкий Codex-бот

Прямое требование: масштабирование на много агентов с первого дня, не Codex как изолированный 2-событийный довесок. Опорная точка — форк `bolzzzz/agent-notifications-go`, который уже реализует ровно эту форму (один Go-бинарь, 5 продуктов — Claude/Codex/OpenCode/CodeBuddy/Cursor — с приоритетным product-detection слоем). Это не абстрактная идея, а изученный вглубь рабочий прецедент (см. `06-multiagent-pivot.md`), который де-рискует направление.

## 2. Архитектурная граница: почему decode → в SDK, а не весь пайплайн в продукте и не всё в SDK

Clean Architecture, направление зависимостей — только внутрь:

| Кольцо | Содержимое | Репозиторий | Почему именно так |
|---|---|---|---|
| Entities/Use Cases | `Event`, classify, dedup, config-резолв, webhook-форматирование | `notification_plugin_go` | Это бизнес-правила ПРОДУКТА уведомлений — генерик SDK не должен знать, что такое «дедуп» или «cooldown» |
| Decode конкретного хоста | Typed payload → структура для каждого агента | `plugin-kit-ai/sdk` | Ровно generic-задача «получить типизированное событие от N хостов» — то, для чего SDK существует, переиспользуемо ЛЮБЫМ потребителем SDK, не только нами |
| Presenters/Frameworks | Desktop notify, click-to-focus, CLI, инсталлятор | `notification_plugin_go` | Платформо-специфичная доставка — не задача SDK |

SRP-обоснование: три пакета — три разных повода для изменения. `hostdetect`/decode в SDK меняется, когда у агента меняется формат payload. Notify-логика в продукте меняется, когда меняется бизнес-правило уведомлений. Инсталлятор меняется, когда меняется способ доставки бинаря. Три разных релизных цикла — совмещать их в одном пакете значило бы плодить ненужные связи между несвязанными причинами изменений.

**Альтернатива, которая была на столе и отклонена**: изначальный Phase 0 (`01-phase0-groundwork.md`) предполагал «hand-roll декодер внутри `notification_plugin_go`, не зависеть от SDK вообще» — чтобы не тащить чужой развивающийся SDK как жёсткую зависимость. Отменено: зависимость от `plugin-kit-ai/sdk` теперь настоящая, но потребляется во время разработки через гитигнорнутый `go.work` (`use (. ../plugin-kit-ai/sdk)`), а не через `replace`-директиву в `go.mod` — `replace` в раннем прототипировании этого же цикла подтверждённо ломает CI/релизную сборку. `go.work` CI не видит вообще, поэтому итоговый `go.mod` обязан резолвиться на настоящий тег SDK, когда фича готова к мержу — единственный момент, когда всё же нужен один реальный релиз на стороне SDK, не «постоянные» релизы на каждый чих.

## 3. Host-detection: где жить логике «кто вызвал хук» — 3 варианта, почему выбран `sdk/hostdetect`

Исследование показало: у SDK `plugin-kit-ai` `Engine.Dispatch` держит **один** `Resolver` на весь `App` — архитектура рассчитана на «один плагин — одна нативная схема вызова для одного хоста», а не на авто-детект среди N хостов под общим argv-контрактом (наш реальный сценарий: один бинарь, `handle-hook <Event> [--product X]`, вызывается разными хостами). `platformmeta` не подходит вообще — это чисто статические метаданные для доков/скаффолдинга (`NativeRoot: "~/.codex/plugins/..."` как строка для документации), без единого байта рантайм-логики.

Три варианта, оценённых по 10-балльной шкале (надёжность/уверенность):

- **(A) `internal/product` внутри `notification_plugin_go`**, по образцу bolzzzz — 8/8, рекомендованный вариант. Обоснование: ни один текущий потребитель SDK не имеет паттерна «один бинарь на N хостов», сам bolzzzz тоже держит это в продукте, а не в общей библиотеке.
- **(B) Новый публичный пакет `sdk/hostdetect`** в SDK, пир к `sdk/codex`/`sdk/claude`/`sdk/platformmeta` — 6/7. Минус: единственный потребитель этого пакета — мы, что раздувает публичную поверхность SDK без второго подтверждённого use case.
- **(C) Встроить в сам `Engine`/`Resolver`** (composable resolver) — 4/5. Технически самое «SDK-native» решение вдолгую, но меняет core dispatch semantics, затрагивает всех текущих потребителей SDK, высокий риск регресса.

**Выбран (B)** — сознательный выбор в пользу размещения в SDK, несмотря на рекомендацию (A).

Согласованный дизайн `plugin-kit-ai/sdk/hostdetect/`:
- Файлы: `hostdetect_claude.go` / `hostdetect_codex.go` / `hostdetect_cursor.go` / `hostdetect_codebuddy.go` / `hostdetect_opencode.go` (каждый — свой `Signal`, расширение = новый файл, не правка существующих — OCP), `signals.go` (`var DefaultRegistry []Signal`, приоритет как у bolzzzz: codebuddy → cursor → codex → claude-default), `detect.go`, `doc.go`.
- API: `type Platform string` (значения `"claude"/"codex"/"cursor"/"codebuddy"/"opencode"` — совпадают с планируемым флагом `--product`, drop-in); `type Env interface { LookupEnv(string) (string, bool) }`; `type Signal struct { Platform Platform; EnvMarkers []string; PayloadSniff func(raw map[string]any) bool }`; `var DefaultRegistry []Signal`; `func Detect(override string, env Env, payload []byte) Platform`.
- `PayloadSniff` намеренно лёгкий — только верхнеуровневые JSON-ключи, НЕ типизированный decode, чтобы избежать риска циклического импорта `sdk/hostdetect ↔ sdk/codex`/`sdk/claude`, если те когда-нибудь захотят самостоятельно регистрироваться.
- В этом заходе реально заполнены только `claude`/`codex` сигналы; `cursor`/`codebuddy`/`opencode` — файлы-заглушки с TODO, форма пакета готова, содержимое — отдельный заход.

## 4. Коллизия имён событий

Прочитан напрямую сгенерированный `sdk/internal/descriptors/gen/resolvers_gen.go`: это **один плоский резолвер на все платформы разом** — `Stop`/`SubagentStop`/`PermissionRequest` уже заняты `Platform:"claude"`. Если добавить Codex-дескрипторы с теми же голыми именами — они будут навсегда затенены записями Claude, а хендлер, зарегистрированный через `app.Codex().OnStop(...)`, молча продиспатчится в Claude-декодер или упадёт с «no handler registered for claude/Stop».

У SDK уже есть готовый прецедент решения именно этой проблемы — Gemini использует префиксованные имена вызова (`GeminiSessionStart`, `GeminiAfterTool`, см. `events_gemini_session.go`), при этом `Event` в дескрипторе остаётся чистым (`"SessionStart"`). Существующий legacy Codex `notify` не коллизирует (голое имя `"notify"` уникально во всём резолвере) — трогать его не нужно.

**Действие**: новые Codex-дескрипторы обязаны использовать `Invocation.Name: "CodexStop"`, `"CodexSubagentStop"`, `"CodexPermissionRequest"` — не голые имена событий. Это уже принятый в SDK паттерн, просто ранее не применённый к Codex.

## 5. Утерянный код — честный отчёт

Искали «почти готовый, отревьюенный SDK-код» новой Codex hooks-системы (Stop/PermissionRequest и т.д.) везде в `plugin-kit-ai`:
- `git fsck --dangling` → 24 висячих коммита, **все** про `agentplugins` (client lifecycle management для Claude/Cline/Gemini/Windsurf/OpenCode-плагинов в разных IDE — отдельная, не относящаяся к делу подсистема) или про bump зависимостей в `landing/`. Ни один не про Codex hooks.
- Все локальные и remote-ветки — единственная многообещающая зацепка, ветка `feat/sdk-codex-lifecycle-hooks` с воркитри `/private/tmp/pkai-codex-lifecycle-hooks`, оказалась ложным следом: сама директория воркитри физически не существует на диске, а `git diff main...feat/sdk-codex-lifecycle-hooks` (94 файла, ~12800 строк) целиком про ту же `agentplugins`-подсистему, ни строчки Codex hooks decode.
- `git stash list` — одна запись, не относится.

**Вывод**: код безвозвратно утерян, писать decode для новой Codex hooks-системы придётся с нуля. Не блокер — все нужные факты уже эмпирически подтверждены живым тестом против реального Codex CLI v0.152.0 (точный wire-формат Stop-события, поля, trust-модель). Просто экономии времени не будет.

Реальное текущее состояние `sdk/codex`: только СТАРЫЙ legacy `notify`-путь (единственное событие `Notify`, argv JSON, `type=agent-turn-complete`). Новой hooks-системы (11 событий, stdin JSON) там нет вообще ни в каком виде.

## 6. Почему инсталлятор для Codex — через нативный marketplace, а не hand-merge hooks.json

Прочитаны напрямую исходники `openai/codex` (`codex-rs/hooks/src/declarations.rs`/`config_rules.rs`): trust-ключ плагин-хука — `plugin_id:relative_path`, **не** command-строка. Доверие переживает обновления плагина, пока не меняются `plugin_id` и относительный путь к hooks-файлу, даже если абсолютный путь к бинарю или логика обёртки меняются между релизами. `PLUGIN_ROOT` резолвится агентом нативно для plugin-bundled хуков (в отличие от Claude Code, который экспортирует только `CLAUDE_PLUGIN_ROOT`).

**Практическое следствие**: для Codex не нужна тяжёлая машинерия из исходного Phase 2 (content-hash TOCTOU-guard, «frozen argv навсегда», pointer-file-костыль для PLUGIN_ROOT) — она остаётся нужна ПОЗЖЕ для Cursor/OpenCode (у Cursor нет marketplace вообще, только сырой `~/.cursor/hooks.json`; у OpenCode — только ручное размещение TS-файла плагина), но не для Codex сейчас.

## 7. Осознанные сужения скоупа этого захода

- **Cursor/OpenCode/CodeBuddy — не в этом заходе.** Архитектура (файл-на-платформу в `hostdetect`, decode-пакет-на-платформу в SDK) уже оставляет для них место (OCP) — расширение не потребует правки существующего кода, просто новый заход.
- **Транскрипт Codex не парсится полностью.** `internal/analyzer`+`pkg/jsonl` заточены под JSONL-схему Claude Code (role/content/tool-use паттерны); у Codex другой формат (`rollout-*.jsonl`). Codex сам отдаёт готовый `last_assistant_message` прямо в payload Stop/SubagentStop — этого достаточно для MVP. Берём этот текст как есть + маленькая новая эвристика классификации статуса, отдельная от полного Claude-анализатора. Богатый Codex-native анализ — отдельный follow-up.
- **`PermissionRequest` вместо полного Notification-паритета.** Ближайший аналог Claude's `Notification` для «нужно внимание пользователя» — но подтверждено живым тестом: НЕ срабатывает под `bypassPermissions`/`dontAsk`/`--ask-for-approval never`. Фиксируем как известное ограничение.
- **`status`/`doctor` кросс-продуктовая команда — не в этом заходе.** Подтверждённо не существует нигде как референс (проверено по всему командному списку форка bolzzzz) — полностью самостоятельная разработка, откладывается.

---

## Дорожная карта реализации Codex-кода (не в этом PR — только план, код появится отдельным PR)

### Stage 1 — SDK: `sdk/hostdetect`
Дизайн — см. раздел 3. Тесты: override всегда побеждает, совпадение по env, совпадение по payload без env, дефолт на claude, порядок приоритета.

### Stage 2 — SDK: Codex hooks decode/encode
Не трогать `sdk/codex/notify.go`/`sdk/internal/platforms/codex/notify.go` (legacy остаётся как есть).
- `sdk/internal/platforms/codex/{types.go,stop.go,subagentstop.go,permissionrequest.go}` — DTO с полями из проверенного payload (`session_id, turn_id, transcript_path, cwd, hook_event_name, model, permission_mode, stop_hook_active, last_assistant_message`, + `tool_name`/`tool_input` для PermissionRequest), по образцу `sdk/internal/platforms/claude/stop.go`. Ack пустой (Codex notification-хуки — пустой stdout + exit 0).
- `sdk/codex/{stop.go,subagentstop.go,permissionrequest.go}` — публичные обёртки по образцу `codex/notify.go`.
- Правка `sdk/internal/descriptors/defs/events_codex.go`: 3 новых `EventDescriptor` в `codexEvents()` — `Invocation.Name: "CodexStop"/"CodexSubagentStop"/"CodexPermissionRequest"` (раздел 4), `Carrier: runtime.CarrierStdinJSON`, `Contract.Maturity: runtime.MaturityBeta`, `Registrar.MethodName: OnStop/OnSubagentStop/OnPermissionRequest`.
- Перегенерировать: `go run ./cmd/plugin-kit-ai-gen` из корня `plugin-kit-ai`, затем `go test ./...` (включая `generator.TestGeneratedArtifactsUpToDate`).
- Обновить `sdk/README.md`/`sdk/STABILITY.md`.
- Тесты: decode-юниты на верифицированный payload + отсутствующие поля + size-guard; App-level интеграционные тесты; **регресс-тест на коллизию** — `gen.ResolveInvocation([]string{"x","Stop"}, env)` должен остаться `Platform:"claude"` после добавления Codex-записей.

### Stage 3 — notification_plugin_go: адаптер к SDK (`internal/codexsource/`)
Единственный пакет продукта, импортирующий SDK (изоляция зависимости, DIP): собирает `pluginkitai.App` на одно обращение, регистрирует хендлер на `app.Codex()`, гоняет `RunContext`, возвращает типизированный decoded-event.

### Stage 4 — рефакторинг `HookData` → `Event`/`EventSource` (самый рискованный шаг)
Механическое извлечение без изменения поведения Claude-пути:
- `internal/hooks/event.go` (новый): `Event`, `EventSource` интерфейс.
- `internal/hooks/claude_source.go`: переносит существующий `HookData`-декодинг без изменения логики.
- `internal/hooks/codex_source.go`: дергает Stage 3, мапит в `Event`, `Message = LastAssistantMessage`.
- `hooks.go`: `Handler` получает `source EventSource`. **`NewHandler(pluginRoot string)` не меняет сигнатуру/поведение** — все существующие вызовы в тестах остаются нетронутыми. Новый `NewHandlerWithSource(pluginRoot, product string, source EventSource)` для composition root. Хелперы переименовываются с `*HookData` на `*Event` — механически, набор полей идентичен.
- `internal/analyzer`: новая `ClassifyLastMessage(text string) Status` — явно проще полного transcript-walk, задокументирована как MVP-упрощение для Codex.
- **Регресс-гейт**: весь существующий `internal/hooks/*_test.go` проходит без единой правки — доказательство, что Claude-путь не изменился.

### Stage 5 — конфиг по продуктам
`internal/config/config.go`: `GetStableConfigDirFor(product)` (`"codex"` → `~/.codex/claude-notifications-go`, default → сегодняшний путь без изменений), `LoadFromPluginRootForProduct(pluginRoot, product)` (legacy-миграция — только для `"claude"`). Существующие `GetStableConfigDir()`/`LoadFromPluginRoot()` остаются тонкими обёртками — точный путь для текущих Claude-пользователей не меняется ни на символ. `internal/config`/`internal/hooks` принимают простую строку `product`, не импортируют `sdk/hostdetect` напрямую — только `main.go` (композиционный корень) знает про SDK.

### Stage 6 — связка в `main.go`
`--product claude|codex` флаг, `io.ReadAll(stdin)` заранее (нужно и для `hostdetect.Detect`, и для передачи в `EventSource`), `hostdetect.Detect(productFlag, env, rawBytes)`, `getPluginRoot()` + фолбэк на `PLUGIN_ROOT` (нативный Codex-экспорт), выбор `EventSource` по платформе, `hooks.NewHandlerWithSource(...)`.

### Stage 7 — инсталлер-артефакты для Codex
`.codex-plugin/plugin.json` (`plugin_id: "claude-notifications-go"`, сохранить навсегда — trust keyed по `plugin_id:relative_path`), `hooks/hooks-codex.json` (отдельное имя файла, не `hooks/hooks.json`, чтобы загрузчик Claude Code его не подхватил). **Открыто**: точная JSON-схема этих файлов не проверена живым тестом (только payload-формат проверен). **Открыто**: bootstrap-путь для Codex-only пользователя без Claude Code не определён — сегодня единственный путь установки бинаря идёт через Claude Code slash-команду.

### Stage 8 — Go 1.22 + релиз SDK (в конце)
`go.mod` `1.21.5→1.22`, обновить матрицы `.github/workflows/ci-{ubuntu,windows,macos}.yml`. Обновление `require github.com/777genius/plugin-kit-ai/sdk` на настоящий тег — только после мержа и тега Stage 2 в `plugin-kit-ai`.

### Верификация (для Stage 1-8, когда до них дойдёт)
- Юнит-тесты обеих сторон (decode, App-level, регресс на коллизию имён; весь текущий `internal/hooks`/`internal/config` — без изменений).
- Песочница: изолированный `CODEX_HOME` (только `auth.json` скопирован, боевые `~/.codex/config.toml`/`hooks.json` не трогаются), реальная Codex-сессия, песочница уничтожается после.
- Ручной smoke: `echo '<эталонный Stop-payload>' | ./claude-notifications handle-hook CodexStop --product codex` с `PLUGIN_ROOT`, проверка текста уведомления.
- Полный прогон существующего тест-сьюта продукта — обязан остаться зелёным без правок в самих тестах Claude-пути.

### Явные нерешённые риски
1. Точная JSON-схема `.codex-plugin/plugin.json`/hooks-конфига не проверена живым тестом.
2. Проводит ли `codex plugin add` через обязательный `/hooks`-trust-review автоматически — не проверено (влияет на UX онбординга, не на работоспособность).
3. Bootstrap-путь для Codex-only пользователя не определён.
4. Плоский платформо-безразличный namespace резолвера SDK — структурное ограничение, повторится для Cursor/OpenCode/CodeBuddy; префиксация (Gemini-стиль) работает, но стоит поднять как архитектурный вопрос на стороне SDK отдельно.
5. Codex получает заведомо более тонкую классификацию статуса, чем Claude — осознанное сужение, не полный паритет с первого дня.
