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

The catalog service address is read from the `CATALOG_URL` environment variable
and defaults to `http://localhost:8081`.

```sh
CATALOG_URL=http://localhost:8081 PORT=8080 go run .
```

Run the tests with `go test ./...`.
