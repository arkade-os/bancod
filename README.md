# bancod

`bancod` is a Go implementation of a **banco solver bot** for the [Arkade](https://arkadeos.com/) virtual mempool.

A *maker* posts a swap offer as a VTXO on an Ark network. The solver bot watches the arkd transaction
stream, finds offers that match its configured pairs and price ranges, and fulfills them atomically
via an introspector-signed Ark transaction.

## Packages

### `pkg/contract`

Protocol primitives for the banco swap. 

- `Offer` — typed representation of a banco swap offer, encoded as a TLV payload
  inside an Ark extension packet (`PacketType = 0x03`). Provides `Serialize`,
  `DeserializeOffer`, `ToPacket`, and `VtxoScript` (builds the swap taproot tree
  from the maker, introspector, and signer keys).
- `CreateOffer` — maker-side helper: queries the introspector for its signer key,
  derives the maker address from the ark client, assembles an `Offer`, and returns
  the hex-encoded offer + extension packet + swap address to fund.
- `GetOffers` — queries the indexer for VTXOs sitting at a swap address, used by
  a maker to check whether its offer is still live.
- `FulfillOffer` — taker-side atomic swap: builds the Ark transaction that spends
  the swap VTXO to the maker's pkScript (paying the `WantAmount`/`WantAsset`) and
  returns change to the taker, signs it with the introspector, and submits it.

### `pkg/solver`

The solver engine. Consumes an offer stream, matches against configured trading
pairs, and drives fulfillment via `pkg/contract`. It's the base building block for taker bot.

- `Solver` — subscribes to `arkd`'s transaction stream, decodes banco offer
  packets out of each `ArkTx`, and fulfills those matching a pair. Exposes
  `Start` / `Stop` / `Status`.
- `Config` — dependencies: `ArkClient`, introspector client, `PairRepository`,
  `PriceFeed`, and an optional `FulfillmentListener`.
- `Pair` — trading pair definition (`base/quote`, min/max amount, decimals,
  price-feed URL, invert flag). `PairRepository` is the read interface used by
  the engine.
- `PriceFeed` / `priceCache` — pluggable price source with an in-memory TTL
  cache (default 5 minutes).
- `FulfillmentEvent` / `FulfillmentListener` — emitted after every successful
  fulfillment; the daemon wires a listener that persists trades to SQLite.

Both `pkg/` packages are intended to be importable by other projects and do
not depend on any `internal/` code.

---

## Binaries

### `bancod`

Daemon that boots a solver, a SQLite-backed wallet, the gRPC+REST API, and the
web UI. Configured entirely through environment variables:

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `BANCOD_ARK_URL` | ✓ | — | arkd gRPC endpoint |
| `BANCOD_WALLET_SEED` | ✓ | — | wallet seed (hex) |
| `BANCOD_INTROSPECTOR_URL` | ✓ | — | introspector endpoint |
| `BANCOD_WALLET_PASSWORD` | | — | wallet unlock password |
| `BANCOD_DATADIR` | | `$HOME/.bancod` | data directory (SQLite DB lives here) |
| `BANCOD_GRPC_PORT` | | `7070` | gRPC listener |
| `BANCOD_HTTP_PORT` | | `7071` | HTTP REST + web UI listener |
| `BANCOD_LOG_LEVEL` | | `4` (Info) | logrus level |

### `banco`

CLI client for the HTTP API. Points at `http://localhost:7071` by default
(`--server` or `BANCO_SERVER` to override). Commands:

```
banco pair add     --pair BTC/<asset> --min … --max … --price-feed …
banco pair update  …
banco pair remove  --pair …
banco pair list
banco status
banco balance
banco address
```

## Building

```sh
make build          # builds ./bancod and ./banco
make docker         # builds the bancod image
make proto          # regenerates api-spec/protobuf/gen
make sqlc           # regenerates internal/infrastructure/db/sqlite/sqlc
make lint
make test           # unit tests
```

## Integration tests

End-to-end tests run against a local nigiri + arkd stack:

```sh
make setup-test-env     # boot nigiri + arkd + introspector, fund arkd wallet
make integrationtest    # run ./test/e2e/...
make teardown-test-env
```
