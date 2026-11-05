# go-library-gateway

The address the browser uses for the library API.

```sh
PORT=8080 go run .
```

| Route | Behaviour |
|---|---|
| `GET /health` | `{"status":"ok"}` when the process is up |

Run the tests with `go test ./...`.
