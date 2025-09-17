## Subscriptions API

Запуск в Docker:

1) Скопируйте `.env.example` в `.env` (при необходимости настройте переменные)
2) объявить билд:

```bash
docker compose up --build
```

После запуска:
- API: `http://localhost:8000`
- Swagger UI: `http://localhost:8000/docs`
- OpenAPI: `http://localhost:8000/openapi.json`

Примеры:
- Создание подписки:

```bash
curl -X POST http://localhost:8000/api/v1/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{
    "service_name": "Yandex Plus",
    "price": 400,
    "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
    "start_date": "07-2025"
  }'
```

- Агрегация за период:

```bash
curl "http://localhost:8000/api/v1/subscriptions/aggregate/total?period_start=01-2025&period_end=12-2025&user_id=60601fee-2bf1-4721-ae6f-7636e79a0cba"
```

Миграции выполняются автоматически при старте контейнера API.


