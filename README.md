# t5s

[![CI](https://github.com/sakusi4/t5s/actions/workflows/ci.yml/badge.svg)](https://github.com/sakusi4/t5s/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Share localhost with only the people you allow.

t5s finds the web services running on your machine, puts a proxy in front of the one you pick, and publishes it at an HTTPS URL through a Cloudflare quick tunnel. Every share has an allowlist of IP addresses or CIDR ranges, and your own public address is always on it. Requests from anywhere else get a 403, and every visitor shows up in the access log on screen.

**Status:** early development. The cloudflared backend and the TUI work. A CLI is planned. Expect breaking changes before v1.

Sharing `dashboard` with one address, then watching who reaches it:

![t5s finds dashboard, shares it with one address and shows the visitors in its access log](docs/demo.gif)

## Install

```sh
brew install -y sakusi4/tap/t5s
```

This also installs [cloudflared](https://github.com/cloudflare/cloudflared), which t5s runs as a subprocess; `-y` accepts that dependency without a prompt. No Cloudflare account is needed.

t5s is developed and tested on macOS. The [releases page](https://github.com/sakusi4/t5s/releases) also carries Linux and Windows builds, but they are untested.

## Usage

```sh
t5s
```

t5s lists the local web services it finds and refreshes the list every 3 seconds. Select one and press `s`, type the addresses to allow, pick an expiry, and press `enter`. The public URL is copied to your clipboard as soon as the tunnel is up.

| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j` | Move |
| `s` | Share the selected service |
| `enter` | Open the access log of a shared service (`esc` goes back) |
| `c` | Copy its URL again |
| `x` | Stop the share |
| `q` | Quit and stop every share |

The allowlist takes IP addresses and CIDR ranges separated by commas or spaces, for example `203.0.113.42, 10.0.0.0/8`. Your own public address is always allowed on top of that, so you can open every share yourself; leave the list empty for a share only you can reach. t5s looks that address up from Cloudflare when it starts and shows it at the bottom right of the screen. Allowing everything (`0.0.0.0/0`, `::/0`) is refused. A share expires after 15 minutes, 1 hour (default), 4 hours or 24 hours, and every share stops when t5s exits.

## How it works

```
Browser
   │ HTTPS
Cloudflare quick tunnel (cloudflared)
   │
t5s local proxy      allowlist · access log · Host rewrite · expiry
   │
localhost:3000
```

Discovery lists the listening TCP ports, keeps the ones that answer HTTP, and names each one from its process (a `next` dev server started in `~/code/dashboard` shows up as `dashboard` / `Next.js`) or from its Docker container.

The proxy decides who gets through. It reads the visitor's address from the `Cf-Connecting-Ip` header that cloudflared sets, checks it against the allowlist, and forwards allowed requests with `Host` rewritten to `localhost:<port>` so local dev servers accept them. HTTP and WebSocket are supported.

The URL is a random `*.trycloudflare.com` hostname. Cloudflare knows nothing about your allowlist, so anyone with the URL reaches the proxy; only the addresses you allowed get past it.

## Try it with sample services

[`hack/compose.yaml`](hack/compose.yaml) starts a few containers to point t5s at:

```sh
docker compose -f hack/compose.yaml up -d
t5s
```

You should see `dashboard` (nginx), `api` and `mail` (Docker). Redis and the SMTP port of `mail` do not answer HTTP, so they stay out of the list. `dashboard` proxies `/api/` to `api`, which echoes the headers it received: share `dashboard`, open the URL from an allowed address, and the page shows the `Host` and `X-Forwarded-For` that t5s set.

## Development

Build from source with Go 1.27 or later:

```sh
git clone https://github.com/sakusi4/t5s.git
cd t5s
go build ./cmd/t5s
./t5s
```

Sharing needs cloudflared on your `PATH` (`brew install cloudflared`).

Every pull request must pass:

```sh
test -z "$(gofmt -l .)"
golangci-lint run
go test -race ./...
go mod tidy -diff
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org/), and pull requests are squash-merged.

## License

[Apache-2.0](LICENSE)
