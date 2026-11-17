package server

import "strings"

// CVSS mapping is deterministic: one vector per vulnerability class, chosen from
// the class alone. The score is computed from the vector, so two runs of the same
// finding always get the same number and a reviewer can audit the vector to
// disagree with it. A finding whose class has no mapping falls back to a vector
// chosen from its severity band, which is the honest thing to do — the alternative
// is a blank field the reader has to guess about.

type cvssMap struct {
	vector string
	// scoreOverride is nil for a genuine v3.1 vector (computed from it). A
	// non-nil value means "no defensible vector, but the severity band implies
	// this base score" and the field is rendered with that caveat.
	scoreOverride *float64
}

func cvssPtr(v float64) *float64 { return &v }

// cvssForClass returns (score string, vector string). Both are empty when there
// is no mapping at all, which the template renders as "not scored".
func cvssForClass(class, severity string) (string, string) {
	c := strings.ToLower(class)
	var m cvssMap
	var ok bool
	m, ok = cvssClassMap[c]
	if !ok {
		// substring guards: class strings vary between agents
		for key, mm := range cvssClassMap {
			if strings.Contains(c, key) {
				m = mm
				ok = true
				break
			}
		}
	}
	if !ok {
		m, ok = cvssSevFallback[strings.ToLower(strings.TrimSpace(severity))]
	}
	if !ok {
		return "", ""
	}
	if m.scoreOverride != nil {
		return cvssFmt(*m.scoreOverride), m.vector
	}
	s := cvssScore(m.vector)
	if s < 0 {
		return "", m.vector
	}
	return cvssFmt(s), m.vector
}

var cvssClassMap = map[string]cvssMap{
	// XSS (reflected & stored): user interaction required, changed scope, low
	// privileges needed for stored. Network, no integrity on host, high impact on
	// victim's browser.
	"cross-site scripting (reflected)": {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N"},
	"cross-site scripting (stored)":    {vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:R/S:C/C:H/I:L/A:N"},
	"cross-site scripting (dom)":       {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N"},

	// CSRF: needs a victim session and a click, no confidentiality impact on the
	// attacker, integrity impact on the victim.
	"cross-site request forgery (csrf)": {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:N/I:H/A:N"},

	// Insecure cookie flags: the finding is exposure of the session to a network
	// attacker who can already see traffic or run script. Adjacent network, no
	// integrity, the score reflects "enables other attacks" rather than a direct
	// compromise.
	"insecure cookie flags": {vector: "CVSS:3.1/AV:A/AC:H/PR:N/UI:R/S:U/C:L/I:N/A:N"},

	// Missing security headers: lowest of the set. No direct compromise, it
	// removes a layer of defence against other attacks.
	"missing security headers": {vector: "CVSS:3.1/AV:N/AC:H/PR:N/UI:R/S:U/C:L/I:L/A:N"},

	// A few classes seen across T1P engagements so the map covers more than
	// this one report.
	"sql injection":            {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
	"command injection":        {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
	"authentication bypass":    {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:L"},
	"idor":                     {vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N"},
	"open redirect":            {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:L/I:N/A:N"},
	"server-side request forgery (ssrf)": {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:N/A:N"},
	"local file inclusion":     {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"},
	"path traversal":           {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"},
	"remote code execution":    {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
	"deserialization":          {vector: "CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:H/A:H"},
	"information disclosure":   {vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N"},
}

var cvssSevFallback = map[string]cvssMap{
	"critical": {vector: "(severity band: critical)", scoreOverride: cvssPtr(9.0)},
	"high":     {vector: "(severity band: high)", scoreOverride: cvssPtr(7.5)},
	"medium":   {vector: "(severity band: medium)", scoreOverride: cvssPtr(5.0)},
	"low":      {vector: "(severity band: low)", scoreOverride: cvssPtr(3.0)},
	"info":     {vector: "(severity band: info)", scoreOverride: cvssPtr(0.0)},
}

// cvssScore computes a v3.1 base score from a vector string. Returns -1 on a
// vector it cannot parse rather than a plausible-looking number.
func cvssScore(v string) float64 {
	m := map[string]string{}
	for _, part := range strings.Split(v, "/") {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		m[kv[0]] = kv[1]
	}
	if m["AV"] == "" || m["AC"] == "" || m["PR"] == "" || m["UI"] == "" ||
		m["S"] == "" || m["C"] == "" || m["I"] == "" || m["A"] == "" {
		return -1
	}
	av := map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2}[m["AV"]]
	ac := map[string]float64{"L": 0.77, "H": 0.44}[m["AC"]]
	prUnchanged := map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}[m["PR"]]
	prChanged := map[string]float64{"N": 0.85, "L": 0.68, "H": 0.5}[m["PR"]]
	ui := map[string]float64{"N": 0.85, "R": 0.62}[m["UI"]]
	cia := map[string]float64{"H": 0.56, "L": 0.22, "N": 0}[m["C"]]
	iia := map[string]float64{"H": 0.56, "L": 0.22, "N": 0}[m["I"]]
	aia := map[string]float64{"H": 0.56, "L": 0.22, "N": 0}[m["A"]]
	var iss, impact float64
	if m["S"] == "U" {
		iss = 1 - (1-cia)*(1-iia)*(1-aia)
		impact = 6.42 * iss
	} else {
		iss = 1 - (1-cia)*(1-iia)*(1-aia)
		impact = 7.52*(iss-0.029) - 3.25*(iss-0.02)*(iss-0.02)*(iss-0.02)
	}
	exploit := 8.22 * av * ac * prUnchanged * ui
	if m["S"] == "C" {
		exploit = 8.22 * av * ac * prChanged * ui
	}
	if impact <= 0 {
		return 0
	}
	if m["S"] == "U" {
		return round1(min3(impact+exploit, 10))
	}
	return round1(min3(1.08*(impact+exploit), 10))
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

func min3(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func cvssFmt(v float64) string {
	// one decimal, matching the CVSS calculator's presentation
	s := round1(v)
	return trimZero(s)
}

func trimZero(v float64) string {
	// format without pulling in strconv for a trailing ".0"
	if v == float64(int(v)) {
		return itoa(int(v)) + ".0"
	}
	// one decimal place is guaranteed by round1
	return itoa(int(v*10)/10) + "." + itoa(int(v*10)%10)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
