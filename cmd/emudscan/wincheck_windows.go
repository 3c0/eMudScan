//go:build windows

package main

import (
	"fmt"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	probeHost = "probe-emudhra.we-have-a-problem.com"
	probeBody = "Your client successfully connected to the site."
)

// runWindowsChecks checks the Windows certificate system. It returns whether the
// chain engine blocks every compromised certificate, and how many test chains a
// program copying the Windows root list (like Python's ssl module) would accept.
// winChecks is the number of Windows checks (the rows of the Windows list).
const winChecks = 8

func runWindowsChecks(c *checker) (allBlocked bool, storeRisk int) {
	c.section(T("log.winStart"))
	w := GroupWindows
	c.begin(w, "windows", T("win.syncName"), "", checkDisallowedSync(c))
	c.begin(w, "windows", T("win.policyName"), "", checkAutoUpdatePolicy(c))
	chain := c.begin(w, "windows", T("win.chainName"), "", checkChainEngine(c))
	allBlocked = chain.Status == StOK
	if c.cancel() {
		return
	}
	live := &liveState{}
	c.begin(w, "windows", T("win.offlineName"), "", checkOfflineTLS(c)) // offline, like the check above
	c.begin(w, "windows", T("win.liveName"), "", checkLiveTLS(c, live))
	c.begin(w, "windows", T("win.inspectName"), "", checkInterception(c, live))
	c.begin(w, "windows", T("win.rootsName"), "", checkRoots(c))
	var risk int
	c.begin(w, "windows", T("win.copyName"), "", checkCopy(c, &risk))
	return allBlocked, risk
}

func filetimeToTime(b []byte) (time.Time, bool) {
	if len(b) < 8 {
		return time.Time{}, false
	}
	ft := syscall.Filetime{LowDateTime: uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24,
		HighDateTime: uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24}
	if ft.HighDateTime == 0 && ft.LowDateTime == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, ft.Nanoseconds()), true
}

func fmtDate(t time.Time) string {
	if Lang == 1 {
		return t.Local().Format("02/01/2006 15:04")
	}
	return t.Local().Format("2006-01-02 15:04")
}

const autoUpdateKey = `SOFTWARE\Microsoft\SystemCertificates\AuthRoot\AutoUpdate`

func checkDisallowedSync(c *checker) func(*Item) {
	return func(it *Item) {
		b, ok := regBinary(hklm, autoUpdateKey, "DisallowedCertLastSyncTime", view64)
		t, okT := filetimeToTime(b)
		r := result{Details: T("win.syncDetails")}
		switch {
		case !ok || !okT:
			r.Status, r.Summary, r.Details = StWarn, T("win.syncNever"), r.Details+"\n\n"+T("win.syncFix")
		case t.Before(microsoftFixDate):
			r.Status, r.Summary, r.Details = StWarn, Tf("win.syncOld", fmtDate(t)), r.Details+"\n\n"+T("win.syncFix")
		default:
			r.Status, r.Summary = StOK, Tf("win.syncOK", fmtDate(t))
		}
		c.finish(it, r)
	}
}

func checkAutoUpdatePolicy(c *checker) func(*Item) {
	return func(it *Item) {
		const pol = `SOFTWARE\Policies\Microsoft\SystemCertificates\AuthRoot`
		var off []string
		if v, ok := regDword(hklm, pol, "DisableRootAutoUpdate", view64); ok && v == 1 {
			off = append(off, "DisableRootAutoUpdate = 1")
		}
		if v, ok := regDword(hklm, pol, "EnableDisallowedCertAutoUpdate", view64); ok && v == 0 {
			off = append(off, "EnableDisallowedCertAutoUpdate = 0")
		}
		if len(off) > 0 {
			c.finish(it, result{Status: StWarn, Summary: T("win.policyOff"), Details: Tf("win.policyOffDetails", `HKLM\`+pol, strings.Join(off, ", "))})
			return
		}
		details := T("win.policyOnDetails")
		if u, ok := regString(hklm, autoUpdateKey, "RootDirUrl", view64); ok && u != "" {
			details += "\n\n" + Tf("win.policyCustomURL", u)
		}
		c.finish(it, result{Status: StOK, Summary: T("win.policyOn"), Details: details})
	}
}

// checkChainEngine asks the Windows chain engine about each of the 16 certificates.
func checkChainEngine(c *checker) func(*Item) {
	return func(it *Item) {
		local := map[string]bool{}
		for _, der := range machineStoreCerts("Disallowed") {
			local[hexSHA256(der)] = true
		}
		blocked, localCount := 0, 0
		var notBlocked, sub []string
		var subSt []Status
		for _, bc := range badCerts {
			st, err := chainErrorStatus(bc.DER)
			label := bc.Name
			if bc.CrossSigned() {
				label += " (Sectigo)"
			}
			if local[bc.SHA256] {
				localCount++
			}
			switch {
			case err != nil:
				notBlocked = append(notBlocked, label+": "+err.Error())
				sub, subSt = append(sub, label+" — "+err.Error()), append(subSt, StError)
			case st&certTrustIsExplicitDistrust != 0:
				blocked++
				sub, subSt = append(sub, label+" — "+T("log.distrusted")), append(subSt, StOK)
			default:
				notBlocked = append(notBlocked, fmt.Sprintf("%s (0x%08X)", label, st))
				sub, subSt = append(sub, fmt.Sprintf("%s — %s (0x%08X)", label, T("log.notDistrusted"), st)), append(subSt, StError)
			}
		}
		details := T("win.chainDetails")
		if localCount > 0 {
			details += "\n\n" + Tf("win.chainLocal", localCount)
		}
		if blocked == len(badCerts) {
			c.finish(it, result{Status: StOK, Summary: Tf("win.chainOK", blocked, len(badCerts)), Details: details})
		} else {
			details += "\n\n" + T("win.chainNotBlocked") + "\n" + bullet(notBlocked) + "\n\n" + T("win.chainFix")
			c.finish(it, result{Status: StError, Summary: Tf("win.chainBad", len(badCerts)-blocked, len(badCerts)), Details: details})
		}
		for i, s := range sub {
			c.line(subSt[i], s)
		}
	}
}

func winhttpErrText(code uint32) string {
	switch code {
	case 12175:
		return T("http.secureFailure")
	case 12007:
		return T("http.dns")
	case 12029, 12030:
		return T("http.connect")
	case 12002:
		return T("http.timeout")
	case 12157:
		return T("http.channel")
	case 12045, 12169, 12170, 12037, 12038:
		return T("http.certError")
	}
	return fmt.Sprintf("Win32 error %d", code)
}

func secureFlagsText(f uint32) string {
	var parts []string
	for _, x := range []struct {
		bit uint32
		key string
	}{{0x4, "flag.revoked"}, {0x8, "flag.invalidCA"}, {0x2, "flag.invalidCert"}, {0x1, "flag.revFailed"},
		{0x10, "flag.cn"}, {0x20, "flag.date"}, {0x40, "flag.usage"}, {0x80000000, "flag.channel"}} {
		if f&x.bit != 0 {
			parts = append(parts, T(x.key))
		}
	}
	if len(parts) == 0 {
		return T("http.secureFailure")
	}
	return strings.Join(parts, ", ")
}

func checkLiveTLS(c *checker, live *liveState) func(*Item) {
	return func(it *Item) {
		if c.offline {
			live.off, live.online, live.accepted = true, false, false
			c.finish(it, result{Name: T("win.liveName"), Status: StInfo, Summary: T("win.liveOff"), Details: T("win.liveOffDetails")})
			return
		}
		live.off = false
		c.activity(Tf("act.connect", probeHost))
		probe := winhttpGet(probeHost, 10000)
		if !probe.Connected || !strings.Contains(probe.Body, probeBody) {
			reason := winhttpErrText(probe.Err)
			if probe.Connected {
				reason = fmt.Sprintf("HTTP %d", probe.Status)
			}
			c.activity("")
			live.online, live.accepted = false, false
			c.finish(it, result{Name: T("win.liveName"), Status: StInfo, Summary: T("win.liveSkipped"), Details: Tf("win.liveSkippedDetails", probeHost, reason)})
			c.line(StInfo, probeHost+": "+reason)
			return
		}
		live.online = true
		var lines []string
		var sts []Status
		accepted := false
		for _, host := range []string{"dv-g3b-emudhra.we-have-a-problem.com", "dv-g3b-aaa-emudhra.we-have-a-problem.com"} {
			c.activity(Tf("act.connect", host))
			r := winhttpGet(host, 10000)
			switch {
			case r.Connected && strings.Contains(r.Body, probeBody):
				accepted = true
				lines, sts = append(lines, Tf("win.liveAccepted", host)), append(sts, StError)
			case r.Connected:
				lines, sts = append(lines, Tf("win.liveError", host, Tf("http.otherServer", r.Status))), append(sts, StInfo)
			case r.Err == 12175 || r.SecureFlags != 0:
				lines, sts = append(lines, Tf("win.liveRejected", host, secureFlagsText(r.SecureFlags))), append(sts, StOK)
			default:
				lines, sts = append(lines, Tf("win.liveError", host, winhttpErrText(r.Err))), append(sts, StInfo)
			}
		}
		c.activity("")
		details := T("win.liveDetails") + "\n\n" + bullet(lines)
		live.accepted = accepted
		if accepted {
			c.finish(it, result{Name: T("win.liveName"), Status: StError, Summary: T("win.liveBad"), Details: details + "\n\n" + T("win.chainFix") + "\n\n" + T("win.liveProxy")})
		} else {
			c.finish(it, result{Name: T("win.liveName"), Status: StOK, Summary: T("win.liveOK"), Details: details})
		}
		c.line(StOK, probeHost+": "+T("log.reachable"))
		for i, l := range lines {
			c.line(sts[i], l)
		}
	}
}

// checkOfflineTLS submits the advisory's test chains, without network, to the checks
// Windows applies to the certificates of a secure connection (see sslChainCheck). It
// does not depend on the test sites, and it checks Windows' own decision on all the
// chains, whatever software inspects secure connections.
func checkOfflineTLS(c *checker) func(*Item) {
	return func(it *Item) { offlineTLSTest(c, it) }
}

func offlineTLSTest(c *checker, it *Item) {
	var lines []string
	var sts []Status
	blocked, total := 0, 0
	// one chain per compromised TLS intermediate and path, named after them (the test
	// sites' names say the same in abbreviations: dv-g3b-aaa…)
	var chains []*testChain
	for _, tc := range testChains {
		if !tc.NoCT && len(tc.Intermediates) > 0 {
			chains = append(chains, tc)
		}
	}
	sort.SliceStable(chains, func(i, j int) bool {
		a, b := chains[i].Intermediates[0].Subject.CommonName, chains[j].Intermediates[0].Subject.CommonName
		return a < b || a == b && !chains[i].AAA && chains[j].AAA
	})
	for _, tc := range chains {
		total++
		label := Tf("chain.direct", tc.Intermediates[0].Subject.CommonName)
		if tc.AAA {
			label = Tf("chain.viaAAA", tc.Intermediates[0].Subject.CommonName)
		}
		var inter [][]byte
		for _, x := range tc.Intermediates {
			inter = append(inter, x.Raw)
		}
		trust, policy, err := sslChainCheck(tc.Host, tc.Leaf.Raw, inter, tc.Leaf.NotBefore.Add(time.Hour))
		switch {
		case err != nil:
			lines, sts = append(lines, Tf("win.offlineError", label, err.Error())), append(sts, StInfo)
		case trust&(certTrustIsExplicitDistrust|certTrustIsRevoked) != 0:
			blocked++
			lines, sts = append(lines, Tf("win.liveRejected", label, T("log.distrusted"))), append(sts, StOK)
		case policy == 0:
			lines, sts = append(lines, Tf("win.offlineAccepted", label)), append(sts, StError)
		default:
			lines, sts = append(lines, Tf("win.offlineNotBlocked", label, policyErrText(policy))), append(sts, StError)
		}
	}
	details := Tf("win.offlineDetails", total/2, total) + "\n\n" + bullet(lines)
	r := result{Name: T("win.offlineName"), Status: StOK, Summary: Tf("win.offlineOK", blocked, total), Details: details}
	if blocked < total {
		r.Status, r.Summary, r.Details = StError, Tf("win.offlineBad", total-blocked, total), details+"\n\n"+T("win.chainFix")
	}
	c.finish(it, r)
	for i, l := range lines {
		c.line(sts[i], l)
	}
}

// policyErrText names the error of Windows' policy for secure connections.
func policyErrText(code uint32) string {
	switch code {
	case 0x800B0109:
		return T("pol.untrustedRoot")
	case 0x800B010C:
		return T("flag.revoked")
	case 0x800B0111:
		return T("log.distrusted")
	case 0x800B0101:
		return T("flag.date")
	case 0x800B010F:
		return T("flag.cn")
	case 0x800B010A:
		return T("pol.chaining")
	}
	return fmt.Sprintf("0x%08X", code)
}

// checkInterception looks for software that inspects secure connections (see
// intercept.go). Its verdict depends on the live test, which goes through it.
func checkInterception(c *checker, live *liveState) func(*Item) {
	return func(it *Item) {
		found := interceptingSoftware(parseCerts(systemStoreCerts("ROOT", nil)))
		switch {
		case len(found) == 0:
			c.finish(it, result{Status: StOK, Summary: T("win.inspectNone"), Details: T("win.inspectNoneDetails")})
		case live.accepted:
			names := strings.Join(found, ", ")
			c.finish(it, result{Status: StWarn, Summary: Tf("win.inspectBy", names), Details: Tf("win.inspectFound", names) + "\n\n" + Tf("win.inspectAccepted", names)})
		case live.online:
			names := strings.Join(found, ", ")
			c.finish(it, result{Status: StInfo, Summary: Tf("win.inspectBy", names), Details: Tf("win.inspectFound", names) + "\n\n" + T("win.inspectRefused")})
		case live.off:
			names := strings.Join(found, ", ")
			c.finish(it, result{Status: StWarn, Summary: Tf("win.inspectBy", names), Details: Tf("win.inspectFound", names) + "\n\n" + Tf("win.inspectOff", names)})
		default:
			names := strings.Join(found, ", ")
			c.finish(it, result{Status: StWarn, Summary: Tf("win.inspectBy", names), Details: Tf("win.inspectFound", names) + "\n\n" + Tf("win.inspectOffline", names)})
		}
	}
}

func checkRoots(c *checker) func(*Item) {
	return func(it *Item) {
		present := affectedRootsIn(parseCerts(systemStoreCerts("ROOT", nil)))
		if len(present) == 0 {
			c.finish(it, result{Status: StOK, Summary: T("win.rootsNone"), Details: T("win.rootsDetails")})
			return
		}
		c.finish(it, result{Status: StInfo, Summary: strings.Join(present, ", "), Details: T("win.rootsDetails")})
	}
}

// checkCopy simulates a program that copies the Windows root list into its own TLS
// library without the block list (what Python's ssl module does).
func checkCopy(c *checker, risk *int) func(*Item) {
	return func(it *Item) {
		keep := func(ctx *syscall.CertContext) bool { return trustedForServerAuth(ctx) }
		certs := parseCerts(append(systemStoreCerts("ROOT", keep), systemStoreCerts("CA", keep)...))
		r := checkCertList(certs)
		*risk = r.Trusted
		if r.Trusted == 0 {
			c.finish(it, result{Status: StOK, Summary: Tf("win.copyOK", tlsChainsTotal), Details: T("win.copyDetails")})
			return
		}
		c.finish(it, result{Status: StCaution, Summary: Tf("win.copyRisk", r.Trusted, tlsChainsTotal),
			Details: T("win.copyDetails") + "\n\n" + Tf("win.copyRiskDetails", r.Trusted, tlsChainsTotal, strings.Join(r.RootsPresent, ", "), fixedVersionsList("python"))})
	}
}
