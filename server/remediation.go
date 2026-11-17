package server

import "strings"

// remediationFor returns a concrete, class-specific fix. "Use parameterized
// queries" is exactly the kind of non-remediation that reads as advice and
// changes nothing, so each class names the actual control: what to stop doing and
// what to do instead. The mapping is deterministic — the same class always gets
// the same guidance, so two reports of one finding agree.

var remediationMap = map[string]string{
	"cross-site scripting (reflected)": "HTML-encode every reflected value by context (HTML body, attribute, JS string, URL) before it is written to the page. Add a strict Content-Security-Policy that blocks inline script as a backstop, not as the fix. Do not rely on escaping alone for attributes — quote them and escape per attribute.",
	"cross-site scripting (stored)":    "Encode on output, not on input: store raw data and escape by context where it is rendered (a snippet shown in HTML, in a title attribute, and in a JSON blob need different escaping). Add a CSP with script-src 'none' or a nonce. Reject known-dangerous sinks if the field is meant to hold markup, and sanitise with an allow-list library before display.",
	"cross-site scripting (dom)":       "Never pass attacker-controlled data to innerHTML, document.write, eval, or a URL that a script reads. Prefer textContent. Where markup is unavoidable, sanitise with DOMPurify or an equivalent allow-list parser before insertion, and add a CSP that blocks inline script.",
	"cross-site request forgery (csrf)": "Require a per-session anti-CSRF token on every state-changing request, bound to the session and checked server-side. Do not accept state changes on GET. Set SameSite=Lax (or Strict where the app allows) on the session cookie as defence-in-depth, and re-authenticate for high-value actions.",
	"insecure cookie flags":             "Set HttpOnly, Secure and SameSite=Lax (Strict where feasible) on every session and auth cookie. Serve the app over HTTPS only and add HSTS so the cookie is never sent over a plaintext hop. Keep the cookie name stable — rotating names invalidates sessions on deploy.",
	"missing security headers":          "Add CSP (script-src 'self' or a nonce; no unsafe-inline), X-Frame-Options DENY or frame-ancestors 'none', X-Content-Type-Options nosniff, Referrer-Policy strict-origin-when-cross-origin, and HSTS with a long max-age behind HTTPS. These are a backstop, not a substitute for fixing the underlying injection or CSRF.",
	"sql injection":                     "Use parameterized queries or prepared statements for every query that touches user input. Never concatenate user input into SQL, and never trust an ORM's raw-query escape hatch for it. Apply least-privilege DB accounts so a successful injection cannot read or write beyond the feature's own tables.",
	"command injection":                 "Do not pass user input to a shell. Where an OS command is unavoidable, use an argv array (exec.Command with separate arguments), avoid shell=True, and validate input against a strict allow-list. Run the worker as an unprivileged user so a successful injection cannot reach the host.",
	"authentication bypass":             "Authenticate on the server for every protected route; never gate access in the client or trust a client-supplied role or user id. Use a vetted auth library, fail closed, and log failures. Test every authenticated endpoint unauthenticated and with a lower-privilege session.",
	"idor":                              "On every object access, check ownership (object.owner == current user or the caller's tenant) in addition to authentication. Never trust an ID from the request as proof of access. Scope DB queries by tenant, and test horizontal (same role, different user) and vertical (low role to high endpoint) access.",
	"open redirect":                     "Do not take redirect targets from request parameters. If a redirect must be dynamic, validate against an allow-list of hosts or restrict to relative paths, and reject anything with a scheme or an @ that changes the authority.",
	"server-side request forgery (ssrf)": "Never fetch a user-supplied URL directly. Allow-list target hosts, resolve DNS and block loopback/RFC1918/link-local/metadata addresses before connecting, and disable redirects. For cloud workloads, require IMDSv2 and block 169.254.169.254 at the network layer.",
	"local file inclusion":              "Map requested names to an allow-list of files instead of passing user input to a filesystem call. Reject path separators and '..' outright, canonicalise the resolved path and confirm it is under the intended directory, and run the service with a read-only filesystem where possible.",
	"path traversal":                    "Reject path separators and '..' in filenames. Join to a base directory, call Clean, and verify the result still has the base directory as a prefix before opening. Serve uploads and static files from a directory the app cannot write to.",
	"remote code execution":             "Remove the code-execution sink reachable from user input (eval, exec, deserialization of untrusted data, template rendering of user-supplied strings). Where a plugin or template must run, sandbox it in a separate process with no network and no credentials. Patch the affected dependency to a fixed version and confirm the vulnerable version is no longer reachable.",
	"deserialization":                   "Never deserialize untrusted data with a polymorphic deserializer (Java ObjectInputStream, .NET BinaryFormatter, Python pickle, Ruby Marshal, YAML with !!python types). Use a data-only format (JSON with a typed schema) and validate fields. Where legacy deserialization is unavoidable, put it behind authentication and an allow-list of classes.",
	"information disclosure":            "Return the minimum the client needs: no stack traces, no internal paths, no version banners, no full objects when a subset was requested. Turn off debug and verbose error modes in production config, and review verbose logs that are exposed to unauthenticated callers.",
	// Asset-lifecycle observations rather than exploitable bugs, but a report
	// still owes the operator a next action or it reads as an unaddressed finding.
	"decommissioned": "Confirm the hostname is genuinely retired and not just unrecorded: check the asset register and the last known owner. If it is retired, remove or mark the DNS record so it cannot be re-registered or squatted, and close the finding as an asset-hygiene correction. If anything still resolves to it, treat that as an orphaned service and route to the owning team before decommissioning.",
	"nxdomain": "Confirm whether the name should still exist. A name that should resolve but returns NXDOMAIN is a broken dependency for anything still calling it — find those callers before deleting. A name that is genuinely retired should be released in the registrar and removed from the asset register so it cannot be re-registered by a third party.",
	"dangling dns": "Identify the decommissioned service the record pointed at and delete or reclaim the DNS record so it cannot be claimed by a third party (subdomain takeover). Update the asset register so the name is not re-pointed at a new service by accident.",
	"subdomain takeover": "Delete or reclaim the dangling DNS record pointing at the unclaimed service, and register the name on the target platform to prevent a third party claiming it. Add the record to monitoring so a future dangling entry is caught before it is exploitable.",
}

// remediationFor looks up the class, then falls back to a substring match, then
// to severity-level guidance when the class is unknown. It returns "" only for a
// finding with no class and no severity, which the template renders as "no
// remediation mapped" rather than inventing generic advice.
func remediationFor(class, severity string) string {
	c := strings.ToLower(class)
	if v, ok := remediationMap[c]; ok {
		return v
	}
	// Substring match first, then keyword match. Class strings vary between
	// agents, so exact keys are not enough: "Session Management / Cookie Flags"
	// must reach the cookie remediation even though the canonical key is
	// "insecure cookie flags".
	for key, v := range remediationMap {
		if strings.Contains(c, key) {
			return v
		}
	}
	for kw, v := range remediationKeywords {
		if strings.Contains(c, kw) {
			return v
		}
	}
	// No class match anywhere: say what the severity band implies rather than
	// invent class-specific advice for a finding whose root cause is unknown.
	return remediationBySeverity(severity)
}

// remediationKeywords catches the class-name variants agents actually emit that a
// substring match on the canonical keys misses.
var remediationKeywords = map[string]string{
	"cookie":         remediationMap["insecure cookie flags"],
	"session":        remediationMap["insecure cookie flags"],
	"security header": remediationMap["missing security headers"],
	"misconfiguration": remediationMap["missing security headers"],
	"xss":            remediationMap["cross-site scripting (reflected)"],
	"cross-site scripting": remediationMap["cross-site scripting (reflected)"],
	"csrf":           remediationMap["cross-site request forgery (csrf)"],
	"cross-site request forgery": remediationMap["cross-site request forgery (csrf)"],
	"sqli":           remediationMap["sql injection"],
	"ssrf":           remediationMap["server-side request forgery (ssrf)"],
	"rce":            remediationMap["remote code execution"],
	"lfi":            remediationMap["local file inclusion"],
	"traversal":      remediationMap["path traversal"],
	"idor":           remediationMap["idor"],
	"redirect":       remediationMap["open redirect"],
	"deserial":       remediationMap["deserialization"],
	"disclosure":     remediationMap["information disclosure"],
	"auth bypass":    remediationMap["authentication bypass"],
	"authentication bypass": remediationMap["authentication bypass"],
}

// remediationBySeverity is the honest fallback: when the class is unknown, say
// what severity implies without pretending to know the fix.
func remediationBySeverity(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical", "high":
		return "No class-specific fix mapped for this finding. Triage as high severity: reproduce it, confirm the impact on a real user or data path, then route to the owning team with the reproduction steps above. Update the finding's vulnerability class once the root cause is known so the specific remediation applies."
	case "medium":
		return "No class-specific fix mapped for this finding. Confirm reproducibility and business impact before scheduling; the reproduction steps above are the input to that triage. Update the finding's vulnerability class once the root cause is known."
	case "low":
		return "No class-specific fix mapped for this finding. Treat as hardening: confirm it is reachable by an attacker in this deployment, then schedule alongside other hardening work. Update the finding's vulnerability class once the root cause is known."
	}
	return ""
}
