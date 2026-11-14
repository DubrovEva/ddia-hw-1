# DDIA HW-2

Мини-фреймворк стратегий устойчивости для обращений к внешнему сервису: default, retries, exponential backoff,
round-robin, hedging и circuit breaking. Пакет `resilience`, Go 1.26, без внешних зависимостей.

## Запуск тестов

```sh
./test.sh
```

Скрипт запускает `go vet`, `go test -race` и, если установлен, `golangci-lint` (конфиг в `.golangci.yml`).

- `resilience/*_test.go` — unit-тесты стратегий на фейковом транспорте `resilience/fake`: сценарий ответов
  задаётся заранее, сеть не используется.
- `integration/` — интеграционные тесты: в каждом тесте поднимается сервер (`httptest.Server`)
  на локальном порту, стратегии ходят к нему через тестовую реализацию `Sender` поверх `net/http`. Тесты
  проверяют в том числе, что отмена подзапроса по таймауту или после победы в hedging доходит до сервера.

## Устройство

Код разделён на два слоя:

- **транспорт** `Sender` отправляет один подзапрос и ничего не знает о повторах и таймаутах;
- **стратегия** `Client` / `MultiClient` решает, куда, когда и сколько раз отправлять подзапросы.

```go
type Sender interface {
	Send(ctx context.Context) (Response, error)
}

type Client interface {
	Do(ctx context.Context, sender Sender) (Response, error)
}

type MultiClient interface {
	Do(ctx context.Context, senders []Sender) (Response, error)
}
```

Таймаут подзапроса порождает стратегия через `ctx`. `Classify` раскладывает результат подзапроса на четыре
исхода: 2xx, 4xx, 5xx, Timeout.

Параметры стратегии и бюджетов передаются в конструктор, транспорты — в `Do` на каждый запрос. Нулевые
значения конфига допустимы: latency budget `0` означает «без ограничения», `Backoff: nil` — повторы без пауз,
балансировщик `nil` — `RoundRobin`.

| Часть задания                | Как собрать                                                    |
|------------------------------|----------------------------------------------------------------|
| default                      | `NewDefault(timeout)`                                          |
| 1. Retries                   | `NewRetry(RetryConfig{Budget, Latency})`                       |
| 2. Exponential backoff       | `NewRetry(RetryConfig{..., Backoff: Exponential{...}})`        |
| 3. Round-robin мультиклиент  | `NewMultiRetry(cfg, RoundRobin{})`                             |
| 4. Хеджирующий мультиклиент  | `NewHedging(delay, total)`                                     |
| 5. Circuit breaking          | `NewMultiRetry(cfg, NewCircuitBreaker(BreakerConfig{...}))`    |

Retries, backoff, round-robin и circuit breaker — это один цикл `MultiRetry`. Отличаются они двумя
подставляемыми зависимостями:

- `Balancer` выбирает реплику для следующего подзапроса, начиная с позиции после предыдущей: `RoundRobin`
  или `CircuitBreaker`;
- `Backoff` задаёт паузу перед повтором: `NoBackoff` или `Exponential`.

Каждая попытка в `MultiRetry` и каждая ветка в `Hedging` выполняется через `Default`.

## Композиция стратегий

`WithClient` превращает клиента вместе с транспортом в новый транспорт, а `PerReplica` делает это для каждой
реплики. Так одну стратегию можно вложить в другую. Например, round-robin, где к каждой реплике применяется
exponential backoff:

```go
backoff := resilience.NewRetry(perReplicaCfg)
roundRobin := resilience.NewMultiRetry(cfg, resilience.RoundRobin{})

resp, err := roundRobin.Do(ctx, resilience.PerReplica(backoff, replicas))
```

Если внутренняя стратегия сдалась, внешняя видит её последний исход, а в итоговой ошибке остаётся ответ
сервера. Исчерпанный latency budget внутренней
стратегии для внешней означает Timeout. Таймаут подзапроса внешней стратегии должен быть не меньше общего
latency budget внутренней, иначе внешняя прервёт внутреннюю посреди её попыток.

## Ошибки

- `*OutcomeError` — последний подзапрос завершился 4xx, 5xx или Timeout, и стратегия сдалась. Поле `Outcome`
  совпадает с исчерпанным бюджетом, в `Response` лежит ответ сервера.
- `ErrLatencyExhausted` — истёк общий latency budget.
- `ErrNoReplicas` — список реплик пуст или circuit breaker выключил их все. Если до этого были попытки,
  ошибка объединена (`errors.Join`) с последним `*OutcomeError`.
- Отмена `ctx` вызывающим кодом возвращается как есть (`context.Canceled` и т. п.).

## Решения, не определённые заданием

- **Бюджет `N` означает «не больше `N` подзапросов с таким исходом».** На `N`-й ошибке этого типа стратегия
  сдаётся. Поэтому бюджеты `0` и `1` ведут себя одинаково.
- **Backoff** ждёт `Base * Factor^(n-1)`, где `n` — число 5xx и Timeout в текущем запросе. После 4xx паузы нет.
  `Max = 0` означает паузу без ограничения сверху. Пауза прерывается, когда истекает latency budget.
- **Hedging, ранний старт.** Если первая реплика ответила ошибкой раньше `hedging_delay`, запросы на остальные
  реплики уходят сразу, как в [gRPC hedging](https://grpc.io/docs/guides/request-hedging/).
- **Hedging, ранний выход.** Если все реплики ответили ошибками, стратегия возвращает ошибку сразу, не дожидаясь
  конца latency budget. Это единственное отступление от буквы задания: ждать больше нечего.
- **Round-robin начинает каждый запрос с первой реплики.** Задание описывает ротацию подзапросов внутри
  запроса, а транспорты одноразовые. Из-за этого первые подзапросы всех запросов приходят на первую реплику.
- **Circuit breaker различает реплики по индексу в списке**, поскольку транспорты одноразовые. Порядок реплик
  между запросами должен быть стабильным.
- **Circuit breaker** выключает реплику на `Cooldown`, если в заполненном окне из последних `Window` подзапросов
  доля ошибок (4xx, 5xx, Timeout) больше `Threshold`. Исходы, пришедшие, пока реплика выключена, не
  учитываются. После `Cooldown` реплика возвращается в ротацию с пустой статистикой. Ротация идёт по включённым
  репликам. `Window` меньше 1 — ошибка конфигурации, конструктор паникует.
- **Retry с circuit breaker** не ждёт паузу backoff, если все реплики выключены, и сразу возвращает
  `ErrNoReplicas`.
