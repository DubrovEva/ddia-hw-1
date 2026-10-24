# DDIA HW-1

CRUD-сервис на Go с метриками ручек, VictoriaMetrics и Grafana.

## Запуск

```sh
make secrets
docker compose up --build
```

`make secrets` один раз генерирует пароль администратора Grafana в
`secrets/grafana_admin_password` (файл не хранится в git). Без него `docker compose up` не запустится.

## Адреса

- приложение: http://localhost:8081, :8082, :8083 (три пода: dc1/pod-1, dc1/pod-2, dc2/pod-1)
- VictoriaMetrics: http://localhost:8428
- Grafana: http://localhost:3000, логин `admin`, пароль из `secrets/grafana_admin_password`
