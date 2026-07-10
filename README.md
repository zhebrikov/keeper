# GophKeeper

Клиент-серверный менеджер паролей на Go с gRPC API и CLI-клиентом.

## Возможности

- Регистрация, аутентификация и авторизация пользователей (JWT + refresh tokens)
- Хранение зашифрованных секретов: логин/пароль, текст, бинарные данные, банковские карты, OTP
- Синхронизация данных между клиентами одного пользователя
- Шифрование данных на клиенте (AES-GCM + PBKDF2) — сервер хранит только зашифрованные blob'ы
- CLI-клиент для Windows, Linux и macOS
- TUI-клиент (`tui`) для интерактивной работы в терминале
- Команда `version` с версией и датой сборки

## Архитектура

```
cmd/client          — точка входа CLI
cmd/server          — точка входа сервера
api/proto           — gRPC/protobuf контракт
internal/
  app/              — bootstrap приложений
  auth/             — JWT
  client/           — gRPC-клиент и локальное хранилище сессии
  tui/              — интерактивный TUI-клиент (Bubble Tea)
  config/           — конфигурация
  crypto/           — шифрование и хеширование
  domain/           — модели и ошибки
  handler/grpc/     — gRPC handlers
  repository/       — слой данных (PostgreSQL)
  service/          — бизнес-логика
```

## Быстрый старт

### Требования

- Go 1.25+
- PostgreSQL 14+
- protoc (для регенерации proto)

### Переменные окружения сервера

| Переменная | Описание | По умолчанию |
|---|---|---|
| `DATABASE_URL` | DSN PostgreSQL | обязательно |
| `JWT_SECRET` | Секрет для подписи JWT | обязательно |
| `GRPC_ADDR` | Адрес gRPC-сервера | `:8080` |
| `TLS_CERT` | Путь к TLS-сертификату сервера | — |
| `TLS_KEY` | Путь к TLS-ключу сервера | — |
| `ACCESS_TOKEN_TTL` | TTL access token | `15m` |
| `REFRESH_TOKEN_TTL` | TTL refresh token | `168h` |

### Переменные окружения клиента

| Переменная | Описание | По умолчанию |
|---|---|---|
| `GOPHKEEPER_SERVER` | Адрес сервера | `localhost:8080` |
| `GOPHKEEPER_CONFIG` | Директория конфигурации | `~/.gophkeeper` |
| `GOPHKEEPER_TLS` | Включить TLS (`true`/`1`) | `false` |
| `GOPHKEEPER_TLS_CA` | Путь к CA-сертификату для проверки сервера | — |
| `GOPHKEEPER_INSECURE` | Отключить TLS (только для разработки) | `false` |

### Сборка

```bash
make build
```

### Запуск сервера

```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/gophkeeper?sslmode=disable"
export JWT_SECRET="your-secret-key"
make tls-certs   # self-signed сертификаты для localhost
make run-server  # запуск с TLS (TLS_CERT/TLS_KEY)
```

Для клиента при TLS-сервере:

```bash
export GOPHKEEPER_TLS=true
export GOPHKEEPER_TLS_CA=certs/ca.crt
./bin/gophkeeper-client login --login alice --password secret123
```

Для локальной разработки без TLS можно запустить сервер без `TLS_CERT`/`TLS_KEY`
и указать клиенту `GOPHKEEPER_INSECURE=true`.

### Использование клиента

```bash
./bin/gophkeeper-client version
./bin/gophkeeper-client register --login alice --password secret123
./bin/gophkeeper-client login --login alice --password secret123
./bin/gophkeeper-client add --type credential --name github --data "user:pass" --metadata "github.com"
./bin/gophkeeper-client list
./bin/gophkeeper-client get <id>
./bin/gophkeeper-client sync
./bin/gophkeeper-client logout
./bin/gophkeeper-client tui
```

### TUI

Интерактивный режим запускается командой `tui`:

```bash
./bin/gophkeeper-client tui
```

Горячие клавиши в списке секретов: `enter` — просмотр, `n` — создать, `e` — редактировать,
`d` — удалить, `s` — синхронизация, `l` — выход из аккаунта, `q` — выход.

## Тестирование

```bash
make test       # все тесты
make coverage   # проверка покрытия >= 70%
```

Покрытие считается по бизнес-пакетам (`internal/*`), без сгенерированного protobuf-кода,
`cmd/*` и CLI-bootstrap (`internal/app/*`).

Интеграционные тесты PostgreSQL:
- автоматически через Docker (testcontainers), если Docker доступен;
- или задайте `DATABASE_URL` для подключения к существующей БД.

## Протокол

gRPC API описан в `api/proto/gophkeeper/v1/gophkeeper.proto`.

Методы:
- `Register`, `Login`, `RefreshToken` — аутентификация
- `CreateSecret`, `UpdateSecret`, `GetSecret`, `ListSecrets`, `DeleteSecret` — CRUD секретов
- `Sync` — инкрементальная синхронизация по `updated_at`

Защищённые методы требуют заголовок `authorization: Bearer <access_token>`.

Конфликты при обновлении обрабатываются через optimistic locking (поле `version`).
