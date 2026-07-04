# Architecture notes

Supplement to README — invariants and extension points.

## Invariants

- Event stream versions are strictly monotonic per `stream_id`
- Inventory `available >= 0`; release requires version captured at reserve (or fresh retry on compensation)
- Saga steps are recorded before advancing; `Resume` skips `done` steps
- Checkout holds inventory before payment — compensating refund is harder than releasing a hold

## Edge cases handled in code

- Parallel checkout on last room: one wins, others get `ErrVersionConflict` or `ErrInsufficientStock`
- Partial multi-night reserve failure: prior nights rolled back in `ReserveNights`
- Payment authorize/void 5xx: `FaultGateway` + checkout `compensation` defer
- Saga void failure: state `failed`, booking stays `confirmed` (no half-cancelled order)
- Duplicate `booking_id`: rejected at orchestrator
- Invalid dates/qty/amount: `ValidateCreate` / `ValidateIDs`

## Not implemented

HTTP/gRPC, Postgres store impl, outbox relay worker, metrics. Interfaces (`Authorizer`, `Holder`, `Store`) are the seam.
