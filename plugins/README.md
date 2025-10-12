# hydr0g3n plugin contract

hydr0g3n can execute external plugins for each result that matches the
configured filters. Plugins run as standalone executables that read a JSON
payload from **stdin** and return a JSON document on **stdout**. Returning a
non-zero exit code indicates a failure and aborts processing of that result.

## Request payload

The JSON document sent to a plugin describes the result that triggered the
plugin:

```json
{
  "url": "https://example.com/admin",
  "method": "GET",
  "status_code": 200,
  "content_length": 1234,
  "duration_ms": 87,
  "body": "... base64 encoded body ...",
  "error": "optional error message"
}
```

* `url` – Absolute URL that hydr0g3n requested.
* `method` – HTTP method used for the request.
* `status_code` – HTTP status code returned by the server. Omitted when the
  request failed.
* `content_length` – Size of the response body in bytes, or `-1` when unknown.
* `duration_ms` – Request latency in milliseconds.
* `body` – Raw response body encoded as base64 (the standard behaviour of Go's
  `encoding/json` for byte slices). The key is omitted when the body is empty.
* `error` – Present only when the request ended in an error and contains the
  error string hydr0g3n recorded.

## Response payload

Plugins respond with a single JSON object. Two actions are supported:

* `verify` – Optional boolean that tells hydr0g3n whether the finding should be
  considered valid. When omitted, hydr0g3n leaves the original match untouched.
* `request` – Optional object that describes a follow-up HTTP request hydr0g3n
  should perform. Fields left empty are ignored.

Example response that asks hydr0g3n to perform a second request with a
different method:

```json
{
  "verify": true,
  "request": {
    "method": "GET",
    "headers": {
      "X-Debug": "true"
    }
  }
}
```

The structure of the `request` object is:

```json
{
  "url": "https://override.example.com/optional",
  "method": "POST",
  "body": "... base64 encoded payload ...",
  "headers": {
    "Header-Name": "value"
  },
  "timeout_ms": 2000,
  "follow_redirects": true
}
```

Plugins may emit diagnostic information to **stderr**. Any additional bytes
written to **stdout** beyond the single JSON document will cause an error.

## Example verifier

The repository ships with an example verifier plugin located at
`plugins/verify_example.py`. The plugin re-fetches the provided URL using a GET
request and verifies the body against a user-supplied regular expression. The
pattern is configured with the `HYDRO_VERIFY_REGEX` environment variable and
defaults to `(?i)success`.

```
HYDRO_VERIFY_REGEX="(?i)example" hydro --plugin ./plugins/verify_example.py ...
```

This script demonstrates the minimal plumbing required to communicate with
hydr0g3n and can serve as a starting point for more advanced integrations.
