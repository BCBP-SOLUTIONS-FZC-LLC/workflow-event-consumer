# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| `main` (latest) | Active |
| Older tags | Not supported |

Only the latest commit on `main` receives security fixes at this stage (pre-release).

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities privately via [GitHub Security Advisories](https://github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/security/advisories/new).

Include:

- A clear description of the vulnerability and the affected component (forwarder, topology loader, SQS consumer wiring, etc.)
- Steps to reproduce
- Impact assessment (event misrouting/leakage across services, token exposure, DoS on a downstream service via redelivery storm)
- Any suggested fix or workaround

### Response timeline

| Step | Target |
|------|--------|
| Initial acknowledgement | 48 hours |
| Severity assessment | 5 business days |
| Patch release (critical/high) | 14 days |
| Public disclosure | After patch ships |

## Scope

Areas of particular sensitivity in this service:

- **`x-internal-token` handling** — each downstream target's token is read once at startup from the environment variable named in `config/topology.yaml`'s `tokenEnv` and never logged; a leak would let an attacker call either downstream service's internal-only API directly.
- **Routing correctness** — `config/topology.yaml`'s `routes` map determines which service sees which event; a misconfiguration could leak an event to a service that shouldn't process it, or silently drop one that only reaches the wrong (broadcast) fallback.
- **AWS IAM permissions** — this service only needs `sqs:ReceiveMessage`/`DeleteMessage`/`ChangeMessageVisibility`/`GetQueueAttributes` on its declared queues and send on their dead-letter queues; it never needs `sns:Publish` or any S3 or Glue permission (see [deploy/aws/policy.json](../deploy/aws/policy.json)). With customer-managed queue keys it also needs decrypt on the source queues' key and generate-data-key and decrypt on the dead-letter queues' key ([deploy/aws/policy-kms.json](../deploy/aws/policy-kms.json)).
- **No PII in logs** — this bridge forwards event payloads verbatim without inspecting them; log lines must never include payload contents, only `event_id`/`type`/status.

## Dependency Vulnerabilities

Dependencies are scanned by `govulncheck` on every CI run and by Dependabot weekly. If you find a vulnerable transitive dependency, please report it via the advisory process above rather than opening a public issue.
