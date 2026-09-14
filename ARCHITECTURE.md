# PortShare Architecture & Product Philosophy

This document outlines the core architectural design and product philosophy for PortShare, specifically detailing our approach to tunneling, privacy, and security.

## The Core Problem: Developer Convenience vs. Enterprise Trust

When developers use PortShare for exposing local endpoints (e.g., Stripe webhooks, OAuth callbacks, API integrations), they face a tension:
- **Convenience**: Terminating TLS at the PortShare gateway gives users an instant `https://your-app.portshare.dev` experience and allows PortShare to inspect, log, and replay HTTP requests.
- **Trust**: If PortShare terminates TLS, the gateway technically has access to plaintext application data (headers, cookies, auth tokens, bodies). For privacy-conscious or enterprise users, "we don't look at your data" is an insufficient guarantee.

To solve this, PortShare's architecture is explicitly divided into three tunneling modes, allowing users to choose the right balance of convenience and privacy.

---

## 1. Standard Mode (HTTP/HTTPS Tunnel)
**Best for:** General web development, webhook testing, frontend client demos.

In this mode, TLS is terminated at the PortShare Gateway. 

```text
Browser
   │
   │ HTTPS
   ▼
PortShare Gateway ─────────────────────┐
   │                                   │
   │ TLS terminated here               │
   │                                   │ Request Inspector
   │ HTTP (Multiplexed)                │ Log & Replay
   ▼                                   │ Header Manipulation
Desktop / CLI Client ──────────────────┘
   │
   ▼
localhost:3000
```

- **Pros:** Instant custom domains, effortless HTTPS, request inspection, replay capabilities.
- **Cons:** HTTP traffic is technically visible to the PortShare infrastructure. We make this explicit to the user.

---

## 2. Private Mode (End-to-End Encrypted Tunnel)
**Best for:** Production API testing, sensitive customer data, compliance-strict environments.

In this mode, PortShare acts as a blind relay. Using **TLS SNI Routing**, the gateway forwards the encrypted connection directly to the client's local machine without decrypting it.

```text
                 TLS
Browser ═══════════════════════════════► Desktop / CLI Client
             encrypted bytes                  │
                    │                   TLS terminates
               Gateway                        │
             cannot decrypt                   ▼
                                         localhost:3000
```

- **Pros:** Zero-knowledge architecture. PortShare cannot see URL paths, headers, cookies, authorization tokens, or request/response bodies.
- **Cons:** Requires the client to manage the TLS certificate (e.g., via a PortShare CA or ACME DNS challenges) to avoid browser warnings.

*Note: PortShare infrastructure can still see connection metadata (IP addresses, SNI, timing, transfer volume).*

---

## 3. Raw TCP Tunnel
**Best for:** SSH, PostgreSQL, Redis, custom game servers (Minecraft), and non-HTTP protocols.

Raw TCP is the foundational transport layer of PortShare. The gateway routes a raw stream of bytes between the public internet and the local machine. PortShare does not understand or inspect the application protocol.

```text
PostgreSQL client
       │
       │ encrypted PostgreSQL/TLS
       ▼
PortShare Gateway
       │
       │ raw bytes
       ▼
PortShare Desktop / CLI Client
       │
       ▼
localhost:5432
```

This model is inherently blind to the application-layer protocol and naturally supports end-to-end encryption if the underlying protocol (e.g., PostgreSQL TLS, SSH) is encrypted.

---

## Strategic Roadmap

To achieve this vision, the development roadmap prioritizes the underlying transport layer before building HTTP-specific features:

1. **The Raw TCP Foundation:** Implement multiplexed raw TCP streams over a persistent encrypted connection (replacing simple HTTP/WS hijacking). This unlocks non-HTTP protocols immediately.
2. **The Trust Dashboard:** Update the user interface to explicitly distinguish between "Standard" (inspectable) and "Private" (E2E) modes, establishing trust through transparency.
3. **True E2E HTTP:** Implement TLS SNI routing at the gateway and solve local certificate provisioning for seamless private HTTPS tunnels.
