# Security Policy

## Reporting A Vulnerability

Please do not open a public issue for a sensitive vulnerability.

Use GitHub Security Advisories or contact the maintainers privately first so the issue can be triaged before disclosure.

## Current Security Scope

CMDR is currently a governance-focused hackathon project and engineering prototype. It contains meaningful safety logic around replay and tool capture, but it is not yet packaged as a hardened multi-tenant production service.

Areas that are currently outside the repo's hardening scope include:

- authentication and authorization for the HTTP API
- tenant isolation and access control
- secret management and rotation
- rate limiting and abuse protection
- deployment-time TLS termination and network policy
- long-term audit retention and operational controls

## What The Project Does Protect

Within its current scope, CMDR is designed to help teams detect and block unsafe agent behavior changes by:

- capturing baseline tool behavior from OTLP evidence
- replaying scenarios against frozen MCP tool responses
- surfacing divergence when a candidate model attempts uncaptured or riskier tool behavior

## Supported Disclosure Window

If a report affects the correctness or safety guarantees of capture, replay, or gate verdicts, please include:

- the affected command or API path
- the expected safety boundary
- how the current behavior violates that boundary
- a minimal reproduction if available

Reports that include a concrete reproduction around drift or frozen replay semantics are the fastest to validate.
