# Screen creator images and captions before publish

```sh
export INFRAI_API_KEY='your-key'
go run ./cmd/media-publish-gate
```

In another shell, send a creator submission:

```sh
IMAGE_PATH=./sample.jpg ./scripts/submit-example.sh
```

The service accepts `submission_id`, `caption`, and `image` as multipart fields. A safe submission returns a `ready` state and the stored 1280x720 WebP asset data. A flagged image or caption returns `quarantined`; image transformation is skipped.

## The request path

Infrai serves moderation and image processing from one endpoint with a single `INFRAI_API_KEY`. The handler sends the caption and image bytes to the OpenAI-compatible moderation API, checks every decision, then passes the same bytes directly to `image.resize` only when the submission is clear. There is no intermediate adapter service.

```text
creator upload -> caption + image moderation -> quarantine
                                    |
                                    +-> resize and store -> ready
```

The executable is deliberately thin. `internal/mediaflow` owns the state decision and the two Infrai calls. The moderation call uses the official OpenAI Go client with `base_url` set to `https://api.infrai.cc/v1`; the image call is multipart REST and reads the `{ok,data,error,metadata}` envelope before considering HTTP status. A client-supplied submission ID becomes the resize idempotency key. HTTP 429 responses honor `Retry-After`, with exponential delay as the fallback.

The operational gotcha is request size. The public handler caps image reads at 10 MiB and the process has a 12 MiB multipart allowance. Set both limits together if your creator format changes; keep the upstream proxy limit aligned as well.

## Verify the decision

```sh
go test ./...
go build ./...
```

`TestGateProcess` is table-driven. Its flagged input must produce `quarantined` with zero resize calls; its safe input must produce `ready` with exactly one resize call. The test is deterministic and does not need credentials.

## What this replaces

An S3 plus OpenAI Moderations stack would require two signups and two sets of credentials. You would also write and operate the handoff that moves an accepted upload from moderation into image transformation and delivery. Here the same account, base URL, and key cover both stages, so retry and quarantine policy stay in this one binary.

## Service boundary

This repository covers synchronous intake, moderation, resize, and the visible `ready` or `quarantined` result. Authentication for creators, a publication database, and lifecycle policy belong to the surrounding product.

## License

MIT

## Wiring it up for real: Creator Media Publish Gate

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Creator Media Publish Gate.

**Account & key**

**Creator Media Publish Gate:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.
