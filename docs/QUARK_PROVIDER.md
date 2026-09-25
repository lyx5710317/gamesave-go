# Quark Netdisk Candidate Provider (Deferred)

Status (2026-09-25): deferred at the owner's request. This is a retained candidate design record, not active evaluation, an experiment, an implementation task, or a release gate. Quark is not supported by the app. Resume work only if the owner explicitly requests it; the official-access entry gate below still applies.

## Entry gate

Development may begin only after official documentation confirms that third-party, open-source Windows native applications can obtain authorized API access. Browser-cookie automation, captured private endpoints, reverse-engineered mobile protocols, or user session-token import are not acceptable foundations.

## Questions requiring official answers

- Can an independent developer or organization apply for production access?
- Is there a documented OpenAPI for file list/stat/upload/download and application directories?
- Are OAuth Client IDs available for installed/native applications?
- Which redirect URI types are accepted for a Windows native client?
- Is PKCE supported, and is a client secret required?
- Is a device-authorization flow available?
- What scopes, consent/review requirements, quotas, API rate limits, and commercial restrictions apply?
- What are the simple and multipart upload limits, part sizing, checksum fields, session expiry, and resume rules?
- Is server-side instant upload officially available, and which content hashes does it require?
- Can downloads resume through standard ranges, and how are temporary URLs renewed?
- Are background token refresh and redistribution of an open-source client permitted?

## Candidate architecture

If access is approved, the adapter should implement the same provider-neutral capabilities used by other cloud providers: auth status, list/stat, upload, download, bounded retry, and disconnect. It must preserve existing OpenSave snapshot, restore, ancestry, and conflict behavior rather than creating a Quark-specific sync engine.

Credentials require Windows-protected storage. A public-client flow is preferred; if Quark requires a confidential client secret, the only acceptable fallback is the separately reviewed minimal OAuth broker described for Baidu. Game saves must move directly between the user's client and Quark and must never pass through that broker.

## Evidence log

Record future primary-source links, access approvals, sandbox findings, confirmed limits, and review dates in this section before changing status from Candidate. Marketing pages, community scripts, and reverse-engineered SDKs do not satisfy the entry gate.
