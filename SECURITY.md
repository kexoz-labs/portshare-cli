# Security Policy

PortShare intentionally exposes local services to the public internet, so we
treat security reports as the highest priority work in this repository.

## Reporting a vulnerability

**Please do not open a public issue, pull request, or discussion for a
vulnerability.** Instead, email **jjagadesh980@gmail.com** with:

- a description of the issue and its impact,
- the affected component (`server`, `cli`, `desktop`, `website`, `admin`),
- reproduction steps or a proof-of-concept,
- any suggested fix or mitigation.

You will get an acknowledgement within **72 hours** and a triage decision
(accepted / needs info / out of scope) within **7 days**. Please give us a
reasonable window to ship a fix before disclosing publicly; we will credit you
in the release notes unless you ask us not to.

## In scope

- Authentication or authorization bypass on tunnels, the admin API, or billing
  endpoints.
- Remote code execution, command injection, or sandbox escape through the
  browser terminal (`pty`) or the SSH/TCP transports.
- Cross-site request or WebSocket hijacking of an authenticated session.
- Leakage of client IDs, API tokens, session cookies, or tunnel contents.
- Rate-limit, bandwidth-quota, or abuse-control bypasses.
- Supply-chain issues in this repository (dependency or workflow compromise).

## Out of scope

- Denial of service through sheer traffic volume against your own tunnel.
- The fact that Standard mode terminates TLS at the gateway: this is documented
  in [ARCHITECTURE.md](./ARCHITECTURE.md) and Private (E2E) mode exists for
  traffic that must stay opaque to the gateway.
- Reports that require a compromised developer machine or a stolen client
  config file.
- Findings from automated scanners with no demonstrated impact.

## Deployment hardening checklist

- Set a long random `SESSION_SECRET` (`openssl rand -hex 32`); the server
  refuses to start with Google OAuth configured but no secret.
- Set `TRUSTED_PROXIES` to your reverse proxy address so `X-Forwarded-*` headers
  cannot be spoofed (they drive per-IP rate limiting and abuse tracking).
- Keep `TLS_CERT`/`TLS_KEY` configured for the TCP and SNI listeners; without
  them the agent handshake is plaintext.
- Restrict `CORS_ORIGINS` and `ADMIN_EMAILS` to your own origins and accounts.
- Never commit `.env` / `.env.production`; only `*.env.example` are tracked.
