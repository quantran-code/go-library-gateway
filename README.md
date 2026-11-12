# go-library-gateway

The address the browser uses for the library API.

```sh
PORT=8080 go run .
```

| Route | Behaviour |
|---|---|
| `GET /health` | `{"status":"ok"}` when the process is up |
| `GET /api/books` | forwarded to `<CATALOG_URL>/books`, returning the catalogue as a JSON array |
| `GET /api/books/{id}` | forwarded to `<CATALOG_URL>/books/{id}`, returning one book or the catalog service's 404 |
| `POST /api/loans` | forwarded to `<LOANS_URL>/loans`, returning `201` with the new loan |
| `GET /api/loans` | forwarded to `<LOANS_URL>/loans`, returning every loan newest first (`[]` when there are none) |
| `GET /api/loans/{id}` | forwarded to `<LOANS_URL>/loans/{id}`, returning one loan or its `404` |
| `POST /api/loans/{id}/return` | forwarded to `<LOANS_URL>/loans/{id}/return`, returning `200` with the returned loan, or the loans service's `400`/`404`/`409`/`502` |

The gateway forwards the loan paths unchanged and does not parse `{id}` itself:
the loans service answers `400 {"error":"invalid loan id"}` for a non-numeric or
non-positive id, and that answer is passed straight back to the caller.

The catalog service address is read from the `CATALOG_URL` environment variable
and defaults to `http://localhost:8081`. The loans service address is read from
`LOANS_URL` and defaults to `http://localhost:8082`.

```sh
CATALOG_URL=http://localhost:8081 LOANS_URL=http://localhost:8082 PORT=8080 go run .
```

Run the tests with `go test ./...`.
