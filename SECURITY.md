# Security policy

Security fixes target the maintained main branch. No published support window or guaranteed response time is promised.

Report privately through [GitHub vulnerability reporting](https://github.com/zzfactor/zzfuse/security/advisories/new) when enabled. Otherwise use the maintainer contact channel listed on their GitHub profile. Do not post credentials or exploit details in public issues. Include versions, locations, reproduction steps, and impact.

Protect management APIs, databases, and embedding services with authenticated access and appropriate TLS. Use separate keys and restricted database roles. Review schema grants: an empty user schema list currently grants unrestricted access.

Python requires configured API keys. Password reset requires verified SMTP delivery; request responses do not disclose accounts or tokens, and token consumption is transactional. See [delivery configuration](docs/configuration.md#password-reset-delivery).

The public repository starts from an audited source snapshot. Original private history and credentials remain in ignored local backups. History exclusion does not revoke old credentials: their owners must rotate any still-active credentials previously committed to the private project. Never push that original history into the public repository.
