package main

// Software that inspects secure connections (antivirus "HTTPS scanning", company
// proxies, debugging proxies) installs its own root certificate in Windows, then
// checks the sites' certificates itself and presents certificates of its own to the
// programs of the PC. Whether the compromised intermediates are blocked then depends
// on that software, not on Windows. They are recognised from their root certificates.

import (
	"crypto/x509"
	"sort"
	"strings"
)

// interceptors: text found in the subject of a root certificate → the software.
var interceptors = []struct{ text, name string }{
	{"kaspersky", "Kaspersky"},
	{"eset ssl filter", "ESET"},
	{"avast web/mail shield", "Avast"},
	{"avast trusted ca", "Avast"},
	{"avg web/mail shield", "AVG"},
	{"avg trusted ca", "AVG"},
	{"norton web/mail shield", "Norton"},
	{"bitdefender personal ca", "Bitdefender"},
	{"dr.web", "Dr.Web"},
	{"adguard personal ca", "AdGuard"},
	{"sophos ssl", "Sophos"},
	{"sophos web appliance", "Sophos"},
	{"zscaler", "Zscaler"},
	{"netskope", "Netskope"},
	{"fortigate", "Fortinet FortiGate"},
	{"fortinet_ca_ssl", "Fortinet FortiGate"},
	{"cisco umbrella", "Cisco Umbrella"},
	{"forcepoint", "Forcepoint"},
	{"check point", "Check Point"},
	{"checkpoint", "Check Point"},
	{"blue coat", "Blue Coat / Symantec proxy"},
	{"barracuda", "Barracuda"},
	{"mcafee web gateway", "McAfee Web Gateway"},
	{"skyhigh", "Skyhigh Security"},
	{"menlo security", "Menlo Security"},
	{"iboss", "iboss"},
	{"lightspeed", "Lightspeed Systems"},
	{"smoothwall", "Smoothwall"},
	{"untangle", "Untangle"},
	{"cloudflare for teams", "Cloudflare Zero Trust"},
	{"gateway ca - cloudflare", "Cloudflare Zero Trust"},
	{"palo alto networks", "Palo Alto Networks"},
	{"do_not_trust_fiddlerroot", "Fiddler"},
	{"charles proxy", "Charles Proxy"},
	{"mitmproxy", "mitmproxy"},
	{"portswigger", "Burp Suite"},
	{"netfilter", "NetFilter SDK (used by some filtering software)"},
}

// interceptingSoftware lists the inspecting software whose root certificates are
// among roots (sorted, without duplicates).
func interceptingSoftware(roots []*x509.Certificate) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range roots {
		subject := lower(strings.Join(append(append([]string{c.Subject.CommonName}, c.Subject.Organization...), c.Subject.OrganizationalUnit...), " | "))
		for _, ic := range interceptors {
			if strings.Contains(subject, ic.text) && !seen[ic.name] {
				seen[ic.name] = true
				out = append(out, ic.name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// liveState is what the live test found, for the checks that depend on it.
type liveState struct {
	off      bool // the online test was turned off
	online   bool // the test sites could be reached
	accepted bool // a test site was accepted
}
