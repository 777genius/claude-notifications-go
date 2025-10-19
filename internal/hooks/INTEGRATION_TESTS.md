# Integration Tests

## 📖 Overview

Эти тесты проверяют **полный цикл работы плагина** без реального Claude Code. Они симулируют реальные сценарии через **mock transcript файлы** и **настоящие HTTP вызовы**.

## 🚀 Быстрый старт

```bash
# Запустить только интеграционные тесты
go test -tags=integration -v ./internal/hooks/

# Запустить конкретный тест
go test -tags=integration -v -run TestE2E_WebhookRetry ./internal/hooks/

# Запустить все тесты (unit + integration)
go test -tags=integration -v ./...
```

## ✅ Что тестируется

### 1. **TestE2E_FullNotificationCycle** (6 секунд)
Полный жизненный цикл сессии с state management.

**Сценарий:**
```
1. PreToolUse: ExitPlanMode
   └─> Notification: plan_ready ✓

2. Notification hook (сразу после)
   └─> Suppressed (cooldown активен) ✓

3. Wait 6 seconds (cooldown истекает)
   └─> Notification: question ✓

4. Stop: task_complete
   └─> Notification: task_complete ✓

5. Cleanup: state files удалены ✓
```

**Проверяет:**
- ✅ State management работает
- ✅ Cooldown suppression работает
- ✅ Session isolation работает
- ✅ Notifications отправляются правильно

---

### 2. **TestE2E_WebhookRetry** (< 1 секунда)
Реальные HTTP вызовы с retry механизмом.

**Сценарий:**
```
HTTP Server → 503 (fail)
            → 503 (fail)
            → 200 (success)

Webhook sender → Retry 3 times ✓
```

**Проверяет:**
- ✅ Retry логика работает
- ✅ Exponential backoff применяется
- ✅ HTTP headers корректные
- ✅ Circuit breaker не мешает (отключен)

---

### 3. **TestE2E_ConcurrentSessions** (< 1 секунда)
Параллельные сессии с изоляцией.

**Сценарий:**
```
Session A: PreToolUse → Stop
Session B: PreToolUse → Stop  (одновременно)
Session C: PreToolUse → Stop  (одновременно)

Результат: 6 notifications (2 на сессию) ✓
```

**Проверяет:**
- ✅ Concurrent access к state файлам
- ✅ Lock механизм работает
- ✅ Изоляция между сессиями

---

## 🎯 Что НЕ требуется

❌ **Реальный Claude Code** - используем mock transcripts
❌ **Графическая среда** - desktop notifications мокаются
❌ **Аудио устройства** - sound playback мокается

## 📊 Результаты

```bash
$ go test -tags=integration -v ./internal/hooks/

=== RUN   TestE2E_FullNotificationCycle
    ✓ Phase 1: plan_ready sent
    ✓ Phase 2: question suppressed
    ✓ Phase 3: notification after cooldown
    ✓ Phase 4: task_complete sent
    ✓ Phase 5: cleanup verified
--- PASS: TestE2E_FullNotificationCycle (6.16s)

=== RUN   TestE2E_WebhookRetry
    Webhook attempt #1
    Webhook attempt #2
    Webhook attempt #3
    ✓ Retry worked (3 attempts)
--- PASS: TestE2E_WebhookRetry (0.52s)

=== RUN   TestE2E_ConcurrentSessions
    ✓ 3 sessions completed
    ✓ 6 notifications sent
--- PASS: TestE2E_ConcurrentSessions (0.14s)

PASS
ok      internal/hooks  7.238s
```

## 🛠️ Архитектура

### Mock Components
- **mockNotifier** - захватывает desktop notifications
- **mockWebhook** - захватывает webhook calls (для некоторых тестов)
- **Real Webhook** - настоящий HTTP sender (для retry тестов)

### Real Components
- **State Manager** - реальные файлы в `/tmp`
- **Dedup Manager** - реальные lock файлы
- **HTTP Server** - настоящий `httptest.Server`

### Transcript Simulation
```go
// Создаем fake transcript как в реальном Claude
transcript := buildTranscriptWithTools(
    []string{"Read", "Edit", "Write"}, // tools
    300,                                 // response length
)
transcriptPath := createTempTranscript(t, transcript)
```

## 🔧 Расширение

### Добавить новый E2E тест:

```go
func TestE2E_MyScenario(t *testing.T) {
    // 1. Setup
    handler, mockNotif, _ := newE2EHandler(t)

    // 2. Create transcript
    transcript := buildTranscriptWithTools(
        []string{"Grep", "Read"},
        250,
    )
    transcriptPath := createTempTranscript(t, transcript)

    // 3. Simulate hook
    hookData := buildHookDataJSON(HookData{
        SessionID:      "test-session",
        TranscriptPath: transcriptPath,
        HookEventName:  "Stop",
    })

    err := handler.HandleHook("Stop", hookData)

    // 4. Verify
    if mockNotif.callCount() != 1 {
        t.Error("Expected 1 notification")
    }
}
```

## 📝 Best Practices

1. **Используйте уникальные session IDs** для каждого теста
2. **Ждите async операций** - webhook.SendAsync нужно время
3. **Проверяйте cleanup** - state/lock файлы должны удаляться
4. **Используйте timeouts** - не позволяйте тестам зависнуть

## 🐛 Troubleshooting

**Тесты медленные?**
```bash
# Запустить только быстрые тесты
go test -tags=integration -v -run "Webhook|Concurrent" ./internal/hooks/
```

**State файлы остаются?**
```bash
# Очистить /tmp
rm -rf /tmp/claude-notifications-*
```

**Webhook не срабатывает?**
- Проверьте что webhook enabled в config
- Увеличьте sleep время для async operations
- Проверьте логи с `-v` флагом

## 📈 CI/CD Integration

```yaml
# .github/workflows/test.yml
- name: Run Integration Tests
  run: |
    go test -tags=integration -v -timeout 30s ./internal/hooks/
```

---

**Вопросы?** Смотрите примеры в `integration_test.go`
