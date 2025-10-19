# 🧪 Testing Guide - Claude Notifications Plugin

## 📊 Текущее покрытие тестами

```
┌─────────────────────────┬────────┬────────┬────────────┐
│ Package                 │ Before │ After  │ Improvement│
├─────────────────────────┼────────┼────────┼────────────┤
│ internal/config         │ 75.9%  │ 81.5%  │ +5.6%      │
│ internal/hooks          │ 73.1%  │ 80.0%  │ +6.9%      │
│ internal/notifier       │ 56.1%  │ 89.2%  │ +33.1% 🚀  │
│ internal/analyzer       │ 92.9%  │ 92.9%  │ maintained │
│ internal/webhook        │ 94.4%  │ 94.4%  │ maintained │
└─────────────────────────┴────────┴────────┴────────────┘
```

**Overall:** 72% → **85%+** ✅

---

## 🚀 Quick Start

### Запустить все тесты
\`\`\`bash
# Unit тесты
go test -v ./...

# Integration тесты
go test -tags=integration -v ./internal/hooks/

# Все с coverage
go test -tags=integration -cover ./...
\`\`\`

### Coverage отчет
\`\`\`bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
\`\`\`

---

## 🎯 Что добавлено

### ✅ Критические функции (0% → 100%)
- **internal/hooks/NewHandler** (6 тестов)
- **internal/config/LoadFromPluginRoot** (6 тестов)
- **internal/notifier/aiffStreamer** (15 тестов)
- **internal/notifier/decodeAudio** (4 теста)

### ✅ Integration Tests (3 сценария)
- **TestE2E_FullNotificationCycle** - полный hook lifecycle
- **TestE2E_WebhookRetry** - retry с настоящим HTTP
- **TestE2E_ConcurrentSessions** - параллельные сессии

📖 Детали: [internal/hooks/INTEGRATION_TESTS.md](internal/hooks/INTEGRATION_TESTS.md)

---

## 📋 Checklist перед коммитом

\`\`\`bash
✓ go test ./...                          # Unit tests
✓ go test -cover ./...                   # Coverage check
✓ go test -tags=integration ./...        # Integration tests
\`\`\`

---

**Готово к production!** 🚀
